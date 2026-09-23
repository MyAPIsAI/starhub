package controller

import (
	"fmt"
	"io"
	"log"
	"net/http"

	"starhub/common"
	"starhub/model"
	"starhub/relay"
	"starhub/relay/channel"
	"starhub/relay/dto"
)

// ---------------------------------------------------------------------------
// Embeddings / Rerank / TTS / STT: OpenAI-format passthrough modes. The
// client body is forwarded verbatim to an OpenAI-compatible channel; the
// mode-specific parsing only feeds validation and billing.
// ---------------------------------------------------------------------------

// RelayEmbeddings serves POST /v1/embeddings.
func RelayEmbeddings(w http.ResponseWriter, r *http.Request) {
	req := relayRequestFromContext(r)
	embeddingReq, err := dto.ParseEmbeddingRequest(req.body)
	if err != nil {
		writeModeError(w, http.StatusBadRequest, "invalid_request_error", "invalid_request_body", err.Error())
		return
	}
	promptEstimate := relay.EstimateTokens(embeddingReq.InputText()) + 1
	preQuota := relay.EstimatePreConsumeTokens(req.modelName, req.billingGroup(), promptEstimate)

	runPassthroughMode(w, r, req, channel.RelayModeEmbeddings, preQuota,
		func(flow *relayFlow, ch *model.Channel, resp *http.Response) attemptResult {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, maxModeResponseBytes))
			resp.Body.Close()
			writePassthroughBody(flow.w, resp, body)
			usage := &relay.Usage{}
			if parsed, err := dto.ParseEmbeddingUsage(body); err == nil && parsed != nil {
				usage.PromptTokens = parsed.PromptTokens
			}
			if usage.PromptTokens == 0 {
				usage.PromptTokens = promptEstimate
			}
			return settleModeAttempt(flow, ch, usage)
		})
}

// RelayRerank serves POST /v1/rerank.
func RelayRerank(w http.ResponseWriter, r *http.Request) {
	req := relayRequestFromContext(r)
	rerankReq, err := dto.ParseRerankRequest(req.body)
	if err != nil {
		writeModeError(w, http.StatusBadRequest, "invalid_request_error", "invalid_request_body", err.Error())
		return
	}
	promptEstimate := relay.EstimateTokens(rerankReq.InputText()) + 1
	preQuota := relay.EstimatePreConsumeTokens(req.modelName, req.billingGroup(), promptEstimate)

	runPassthroughMode(w, r, req, channel.RelayModeRerank, preQuota,
		func(flow *relayFlow, ch *model.Channel, resp *http.Response) attemptResult {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, maxModeResponseBytes))
			resp.Body.Close()
			writePassthroughBody(flow.w, resp, body)
			usage := &relay.Usage{PromptTokens: promptEstimate}
			if parsed, err := dto.ParseRerankUsage(body); err == nil && parsed != nil {
				if parsed.PromptTokens > 0 {
					usage.PromptTokens = parsed.PromptTokens
				}
				usage.CompletionTokens = parsed.CompletionTokens
			}
			return settleModeAttempt(flow, ch, usage)
		})
}

// RelayTTS serves POST /v1/audio/speech. The response is audio bytes: no
// usage report exists, so the settlement keeps the input-token estimate
// (billed-what-was-asked semantics).
func RelayTTS(w http.ResponseWriter, r *http.Request) {
	req := relayRequestFromContext(r)
	if _, err := dto.ParseTTSRequest(req.body); err != nil {
		writeModeError(w, http.StatusBadRequest, "invalid_request_error", "invalid_request_body", err.Error())
		return
	}

	// Pre-charge: per-call price or the input-token estimate.
	pd := relay.NewPriceDataWithUserGroup(req.modelName, req.billingGroup(), req.userGroup)
	usage := &relay.Usage{}
	if !pd.IsPerCall() && !pd.IsExpression() {
		if ttsReq, err := dto.ParseTTSRequest(req.body); err == nil {
			usage.PromptTokens = relay.EstimateTokens(ttsReq.Input)
		}
	}
	preQuota := relay.ComputeUsageQuota(pd, usage)
	if preQuota < 1 {
		preQuota = 1
	}

	runPassthroughMode(w, r, req, channel.RelayModeTTS, preQuota,
		func(flow *relayFlow, ch *model.Channel, resp *http.Response) attemptResult {
			// Stream the audio through; the upstream may take a while.
			contentType := resp.Header.Get("Content-Type")
			if contentType == "" {
				contentType = "audio/mpeg"
			}
			flow.w.Header().Set("Content-Type", contentType)
			flow.w.WriteHeader(http.StatusOK)
			size, err := io.Copy(flow.w, io.LimitReader(resp.Body, maxModeResponseBytes))
			resp.Body.Close()
			if err != nil && size == 0 {
				return attemptResult{outcome: attemptUpstreamError,
					cause: fmt.Errorf("copy speech response: %v", err)}
			}
			// No usage report on the TTS path: bill the estimate (the
			// input text is the billed quantity).
			return settleModeAttempt(flow, ch, usage)
		})
}

// RelaySTT serves POST /v1/audio/transcriptions (multipart).
func RelaySTT(w http.ResponseWriter, r *http.Request) {
	relayAudioTranscription(w, r, channel.RelayModeSTT)
}

// RelaySTTTranslations serves POST /v1/audio/translations (multipart).
func RelaySTTTranslations(w http.ResponseWriter, r *http.Request) {
	relayAudioTranscription(w, r, channel.RelayModeSTTTranslation)
}

func relayAudioTranscription(w http.ResponseWriter, r *http.Request, mode channel.RelayMode) {
	req := relayRequestFromContext(r)
	form, err := common.ParseMultipartFormMeta(req.contentType, req.body)
	if err != nil {
		writeModeError(w, http.StatusBadRequest, "invalid_request_error", "invalid_request_body", err.Error())
		return
	}
	if !form.HasFile("file") {
		writeModeError(w, http.StatusBadRequest, "invalid_request_error", "missing_file",
			"the \"file\" multipart field is required")
		return
	}

	// STT billing: per-call price when configured, otherwise audio tokens
	// estimated from the file size (verbose_json responses carry the exact
	// duration at settle time).
	pd := relay.NewPriceDataWithUserGroup(req.modelName, req.billingGroup(), req.userGroup)
	usage := &relay.Usage{}
	if !pd.IsPerCall() && !pd.IsExpression() {
		usage.PromptTokens = estimateAudioTokens(form.FileBytes())
	}
	preQuota := relay.ComputeUsageQuota(pd, usage)
	if preQuota < 1 {
		preQuota = 1
	}

	runPassthroughMode(w, r, req, mode, preQuota,
		func(flow *relayFlow, ch *model.Channel, resp *http.Response) attemptResult {
			contentType := resp.Header.Get("Content-Type")
			if common.AssertJSONContentType(contentType) {
				body, _ := io.ReadAll(io.LimitReader(resp.Body, maxModeResponseBytes))
				resp.Body.Close()
				writePassthroughBody(flow.w, resp, body)
				if parsed, perr := dto.ParseTranscriptionResponse(body); perr == nil {
					if usage.PromptTokens == 0 || parsed.Duration > 0 {
						if parsed.Duration > 0 {
							// 1 audio token ~ 0.04s (25 tokens per second).
							usage.PromptTokens = int64(parsed.Duration*25 + 0.5)
						}
					}
				}
				return settleModeAttempt(flow, ch, usage)
			}
			// Audio passthrough (response_format on some providers).
			flow.w.Header().Set("Content-Type", contentType)
			flow.w.WriteHeader(http.StatusOK)
			_, _ = io.Copy(flow.w, io.LimitReader(resp.Body, maxModeResponseBytes))
			resp.Body.Close()
			return settleModeAttempt(flow, ch, usage)
		})
}

const maxModeResponseBytes = 64 << 20

// readBoundedBody reads up to limit bytes of an upstream response into a
// buffer. Content-Length is only a hint; short or longer bodies are still
// read up to the limit. Reading any tail separately avoids doubling a full
// body buffer just to discover EOF. Unknown lengths use Go's chunked ReadAll.
func readBoundedBody(resp *http.Response, limit int64) []byte {
	if limit <= 0 {
		return nil
	}
	if resp.ContentLength > 0 && resp.ContentLength <= limit {
		body := make([]byte, resp.ContentLength)
		n, err := io.ReadFull(resp.Body, body)
		body = body[:n]
		if err != nil || int64(n) == limit {
			return body
		}
		tail, _ := io.ReadAll(io.LimitReader(resp.Body, limit-int64(n)))
		return append(body, tail...)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, limit))
	return body
}

// estimateAudioTokens approximates the audio token count from the uploaded
// bytes. Audio tokens run at ~25/second; assuming a 128kbps stream (~16
// KB/s) this over-reserves for higher-bitrate files, the safe direction
// for a pre-charge (settle corrects with the verbose_json duration).
func estimateAudioTokens(fileBytes int64) int64 {
	if fileBytes <= 0 {
		return 1
	}
	tokens := fileBytes / 640
	if tokens < 1 {
		tokens = 1
	}
	return tokens
}

// modeLabel maps a RelayMode to the short metric label embedded in the
// request span.
func modeLabel(mode channel.RelayMode) string {
	switch mode {
	case channel.RelayModeEmbeddings:
		return "embeddings"
	case channel.RelayModeRerank:
		return "rerank"
	case channel.RelayModeTTS:
		return "tts"
	case channel.RelayModeSTT, channel.RelayModeSTTTranslation:
		return "stt"
	case channel.RelayModeImageGeneration, channel.RelayModeImageEdit:
		return "image"
	}
	return "passthrough"
}

// runPassthroughMode wraps the shared flow for the passthrough modes:
// validation happened above, the pre-charge is explicit, and every attempt
// replays the original body (multipart bodies keep their content-type).
func runPassthroughMode(w http.ResponseWriter, r *http.Request, req relayRequest,
	mode channel.RelayMode, preQuota int64, handle func(flow *relayFlow, ch *model.Channel, resp *http.Response) attemptResult) {

	if req.modelName == "" {
		writeModeError(w, http.StatusInternalServerError, "server_error", "internal_error", "request model is missing")
		return
	}

	pre, err := relay.PreConsumeQuotaAmount(req.userID, req.tokenID, req.tokenUnlimited, preQuota)
	if err != nil {
		writePreConsumeError(w, "", err)
		return
	}

	flow := newRelayFlow(r.Context(), w, req, "", modeLabel(mode))
	flow.preQuota = pre
	defer flow.refundUnlessSettled()
	defer flow.finish()

	flow.runChannels(func(ch *model.Channel) attemptResult {
		adaptor, err := relay.GetChatAdaptor(ch.Type)
		if err != nil {
			return attemptResult{outcome: attemptUpstreamError,
				cause: fmt.Errorf("channel %d: %v", ch.ID, err)}
		}
		url, err := adaptor.ModeURL(ch.BaseURL, mode)
		if err != nil {
			return attemptResult{outcome: attemptUnsupported, cause: err}
		}
		key, keyIdx := ch.SelectKey()
		resp, fail := flow.callUpstream(ch, adaptor, upstreamCall{
			method:      http.MethodPost,
			url:         url,
			contentType: req.contentType,
			body:        req.body,
			key:         key,
			keyIdx:      keyIdx,
		})
		if fail != nil {
			return *fail
		}
		return handle(flow, ch, resp)
	})
}

// settleModeAttempt balances a successful passthrough attempt.
func settleModeAttempt(flow *relayFlow, ch *model.Channel, usage *relay.Usage) attemptResult {
	if usage == nil {
		usage = &relay.Usage{}
	}
	if err := relay.PostConsumeQuotaWithUserGroup(flow.req.userID, flow.req.tokenID, flow.req.tokenName,
		flow.req.tokenUnlimited, ch.ID, flow.req.modelName, flow.settleGroup(), flow.req.userGroup, usage,
		flow.preQuota, flow.trailString()); err != nil {
		log.Printf("post consume quota failed: %v", err)
	}
	flow.settled = true
	return attemptResult{outcome: attemptSuccess, statusCode: http.StatusOK, usage: usage}
}

func writePassthroughBody(w http.ResponseWriter, resp *http.Response, body []byte) {
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func writeModeError(w http.ResponseWriter, status int, errType, code, message string) {
	dto.WriteRelayError(w, status, dto.InboundOpenAI, errType, code, message)
}

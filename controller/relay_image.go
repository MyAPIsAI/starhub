package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"starhub/common"
	"starhub/model"
	"starhub/relay"
	"starhub/relay/channel"
	"starhub/relay/dto"
)

// ---------------------------------------------------------------------------
// Image relay (P3.1): generations (JSON) and edits (multipart), per-image
// billing (size x quality x count), streamed images as SSE.
// ---------------------------------------------------------------------------

// RelayImageGenerations serves POST /v1/images/generations.
func RelayImageGenerations(w http.ResponseWriter, r *http.Request) {
	req := relayRequestFromContext(r)
	imgReq, err := dto.ParseImageRequest(req.body)
	if err != nil {
		writeModeError(w, http.StatusBadRequest, "invalid_request_error", "invalid_request_body", err.Error())
		return
	}
	if imgReq.Stream == nil {
		imgReq.Stream = &req.stream
	}
	relayImage(w, r, req, channel.RelayModeImageGeneration, imgReq, req.body, req.contentType)
}

// RelayImageEdits serves POST /v1/images/edits (multipart). The raw body is
// replayed verbatim on every attempt; the parsed form feeds validation and
// billing only.
func RelayImageEdits(w http.ResponseWriter, r *http.Request) {
	req := relayRequestFromContext(r)
	form, err := common.ParseMultipartForm(req.contentType, req.body)
	if err != nil {
		writeModeError(w, http.StatusBadRequest, "invalid_request_error", "invalid_request_body", err.Error())
		return
	}
	if form.File("image") == nil {
		writeModeError(w, http.StatusBadRequest, "invalid_request_error", "missing_image",
			"the \"image\" multipart field is required")
		return
	}
	imgReq := &dto.ImageRequest{
		Model:   form.Value("model"),
		Prompt:  form.Value("prompt"),
		Size:    form.Value("size"),
		Quality: form.Value("quality"),
		Extra:   map[string]json.RawMessage{},
	}
	if raw := form.Value("n"); raw != "" {
		n, err := strconv.ParseUint(raw, 10, 32)
		if err != nil {
			writeModeError(w, http.StatusBadRequest, "invalid_request_error", "invalid_request_body",
				fmt.Sprintf("invalid n %q: must be an integer", raw))
			return
		}
		// Same uint shape as the JSON path: a wrapped-around value stays a
		// wrapped number and the [1, MaxImageN] check catches it.
		nVal := uint(n)
		imgReq.N = &nVal
	}
	if raw := form.Value("stream"); raw != "" {
		stream := raw == "true" || raw == "1"
		imgReq.Stream = &stream
	}
	if imgReq.Model == "" {
		writeModeError(w, http.StatusBadRequest, "invalid_request_error", "model_required",
			"the \"model\" form field is required")
		return
	}
	if imgReq.Prompt == "" {
		writeModeError(w, http.StatusBadRequest, "invalid_request_error", "prompt_required",
			"the \"prompt\" form field is required")
		return
	}
	// The multipart n must pass the same bound as the JSON path (both
	// parsing paths share one billing gate, P3.1).
	if err := imgReq.ValidateN(); err != nil {
		writeModeError(w, http.StatusBadRequest, "invalid_request_error", "invalid_request_body", err.Error())
		return
	}
	if err := imgReq.ValidateSize(); err != nil {
		writeModeError(w, http.StatusBadRequest, "invalid_request_error", "invalid_request_body", err.Error())
		return
	}
	// The distributor extracted the model from the form; the image request
	// is authoritative for the rest.
	req.stream = imgReq.Stream != nil && *imgReq.Stream
	relayImage(w, r, req, channel.RelayModeImageEdit, imgReq, req.body, req.contentType)
}

func relayImage(w http.ResponseWriter, r *http.Request, req relayRequest,
	mode channel.RelayMode, imgReq *dto.ImageRequest, outBody []byte, contentType string) {

	stream := imgReq.Stream != nil && *imgReq.Stream
	bill := &relay.ImageBill{
		Model:   imgReq.Model,
		Group:   req.group,
		Size:    imgReq.Size,
		Quality: imgReq.Quality,
		Prompt:  imgReq.Prompt,
		Count:   int(imgReq.Count()),
	}

	pre, err := relay.PreConsumeQuotaAmount(req.userID, req.tokenID, req.tokenUnlimited, bill.PreQuota())
	if err != nil {
		writePreConsumeError(w, "", err)
		return
	}

	flow := newRelayFlow(r.Context(), w, req, "")
	flow.preQuota = pre
	defer flow.refundUnlessSettled()

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
			contentType: contentType,
			body:        outBody,
			key:         key,
			keyIdx:      keyIdx,
		})
		if fail != nil {
			return *fail
		}
		if stream {
			return imageStreamAttempt(flow, ch, resp, bill)
		}
		return imageJSONAttempt(flow, ch, resp, bill)
	})
}

// imageJSONAttempt relays a non-stream image response verbatim and settles
// on the delivered count.
func imageJSONAttempt(flow *relayFlow, ch *model.Channel, resp *http.Response, bill *relay.ImageBill) attemptResult {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxModeResponseBytes))
	resp.Body.Close()
	writePassthroughBody(flow.w, resp, body)

	count := 0
	var usage *dto.ImageUsage
	if parsed, err := dto.ParseImageResponse(body); err == nil {
		count = parsed.Count()
		usage = parsed.Usage
	}
	actual := bill.SettleQuota(count, usage)
	settleImageAttempt(flow, ch, actual)
	return attemptResult{outcome: attemptSuccess}
}

// imageStreamAttempt relays a streamed image response (partial_image
// previews are forwarded free; completed images bill one by one) and
// enforces the two billing invariants of P3.1:
//
//  1. a client disconnect must not lower the billed count — the upstream
//     stream is drained to EOF even after writes fail;
//  2. a completed image arriving after the disconnect must raise it.
//
// Upstreams that answer a stream request with plain JSON are wrapped into
// the SSE format (the gateway's protocol promise outranks the upstream's).
func imageStreamAttempt(flow *relayFlow, ch *model.Channel, resp *http.Response, bill *relay.ImageBill) attemptResult {
	isSSE := strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream")
	if !isSSE {
		return imageJSONasSSEAttempt(flow, ch, resp, bill)
	}

	flow.w.Header().Set("Content-Type", "text/event-stream")
	flow.w.Header().Set("Cache-Control", "no-cache")
	flow.w.WriteHeader(http.StatusOK)
	flusher, _ := flow.w.(http.Flusher)

	reader := channel.NewSSEReader(resp.Body)
	defer resp.Body.Close()

	completed := 0
	var streamErr string
	var streamUsage *dto.ImageUsage
	for {
		line, err := reader.Next()
		if err != nil {
			if err != io.EOF {
				log.Printf("image stream read error: %v", err)
			}
			break
		}
		if line.Event != "" {
			fmt.Fprintf(flow.w, "event: %s\n", line.Event)
		}
		if line.Data != "" {
			fmt.Fprintf(flow.w, "data: %s\n", line.Data)
		}
		fmt.Fprint(flow.w, "\n")
		if flusher != nil {
			flusher.Flush()
		}
		if line.Data == "" {
			continue
		}
		ev, perr := dto.ParseImageStreamEvent([]byte(line.Event), []byte(line.Data))
		if perr != nil {
			continue
		}
		if ev.Usage != nil {
			streamUsage = ev.Usage
		}
		switch {
		case ev.Type == "image_generation.completed":
			completed++
		case ev.Type == "error" && ev.ErrorMessage != "":
			streamErr = ev.ErrorMessage
		}
	}

	// Nothing delivered and the stream errored: nothing settles, the
	// deferred refund returns the whole pre-payment.
	if completed == 0 && streamErr != "" {
		return attemptResult{outcome: attemptSuccess}
	}
	actual := bill.SettleQuota(completed, streamUsage)
	settleImageAttempt(flow, ch, actual)
	return attemptResult{outcome: attemptSuccess}
}

// imageJSONasSSEAttempt wraps a JSON image response into SSE completed
// events (one per image) when the client asked for a stream but the
// upstream does not stream.
func imageJSONasSSEAttempt(flow *relayFlow, ch *model.Channel, resp *http.Response, bill *relay.ImageBill) attemptResult {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxModeResponseBytes))
	resp.Body.Close()

	var parsed *dto.ImageResponse
	if p, err := dto.ParseImageResponse(body); err == nil {
		parsed = p
	}
	if parsed == nil || parsed.Count() == 0 {
		// Unparseable / empty: forward the raw JSON unchanged (not SSE) —
		// better a working answer in the wrong shape than data loss.
		writePassthroughBody(flow.w, resp, body)
		actual := bill.SettleQuota(0, nil)
		settleImageAttempt(flow, ch, actual)
		return attemptResult{outcome: attemptSuccess}
	}

	flow.w.Header().Set("Content-Type", "text/event-stream")
	flow.w.Header().Set("Cache-Control", "no-cache")
	flow.w.WriteHeader(http.StatusOK)
	flusher, _ := flow.w.(http.Flusher)
	for _, img := range parsed.Data {
		payload, _ := json.Marshal(map[string]any{
			"type":         "image_generation.completed",
			"output_index": 0,
			"b64_json":     img.B64JSON,
			"url":          img.URL,
		})
		fmt.Fprintf(flow.w, "event: image_generation.completed\ndata: %s\n\n", payload)
		if flusher != nil {
			flusher.Flush()
		}
	}
	if parsed.Usage != nil {
		payload, _ := json.Marshal(map[string]any{"usage": parsed.Usage})
		fmt.Fprintf(flow.w, "data: %s\n\n", payload)
		if flusher != nil {
			flusher.Flush()
		}
	}
	fmt.Fprint(flow.w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
	actual := bill.SettleQuota(parsed.Count(), parsed.Usage)
	settleImageAttempt(flow, ch, actual)
	return attemptResult{outcome: attemptSuccess}
}

// settleImageAttempt balances the image bill (unit-based: the amount was
// computed by ImageBill with the count/usage guards).
func settleImageAttempt(flow *relayFlow, ch *model.Channel, actual int64) {
	if err := relay.PostConsumeQuotaAmount(flow.req.userID, flow.req.tokenID, flow.req.tokenName,
		flow.req.tokenUnlimited, ch.ID, flow.req.modelName, actual, flow.preQuota, flow.trailString()); err != nil {
		log.Printf("post consume image quota failed: %v", err)
	}
	flow.settled = true
}

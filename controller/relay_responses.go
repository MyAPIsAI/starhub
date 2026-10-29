package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	"starhub/common"
	"starhub/model"
	"starhub/relay"
	"starhub/relay/channel"
	"starhub/relay/dto"
	"starhub/service"
)

// RelayResponses forwards native foreground Responses JSON and SSE while
// retaining the gateway's authorization, reservation and settlement flow.
func RelayResponses(w http.ResponseWriter, r *http.Request) {
	req := relayRequestFromContext(r)
	parsed, err := dto.ParseResponsesRequest(req.body)
	if parsed != nil {
		parsed.Compact = r.URL.Path == "/v1/responses/compact"
	}
	if err == nil {
		err = relay.ValidateResponsesRequest(parsed)
	}
	if err != nil {
		writeModeError(w, 400, "invalid_request_error", "invalid_request_body", err.Error())
		return
	}
	if parsed.Compact && parsed.Stream {
		writeModeError(w, 400, "invalid_request_error", "invalid_request_body", "compaction does not support streaming")
		return
	}
	if ref, ok := r.Context().Value(common.CtxResponseReference).(*model.ResponseReference); ok {
		if ref.PromptTokens+ref.CompletionTokens > math.MaxInt64-parsed.InputTokens {
			writeModeError(w, 400, "invalid_request_error", "context_length_exceeded", "stored response context exceeds the gateway limit")
			return
		}
		parsed.InputTokens += ref.PromptTokens + ref.CompletionTokens
	}
	previousCheck := req.channelCheck
	req.channelCheck = func(ch *model.Channel, group string) (bool, error) {
		if previousCheck != nil {
			ok, err := previousCheck(ch, group)
			if err != nil || !ok {
				return ok, err
			}
		}
		adaptor, err := relay.GetChatAdaptor(ch.Type)
		if err != nil {
			return false, nil
		}
		_, err = adaptor.ModeURL(ch.BaseURL, channel.RelayModeResponses)
		return err == nil, nil
	}
	var lease *model.ResponseConversationLease
	if conversation, ok := r.Context().Value(common.CtxResponseConversation).(*model.ResponseConversation); ok {
		lease = &model.ResponseConversationLease{Conversation: conversation, Nonce: common.GenerateSessionKey()}
		r = r.WithContext(common.WithValues(r.Context(), common.CtxResponseConversationLease, lease))
	}
	flow := newRelayFlow(r.Context(), w, req, dto.InboundOpenAI, "responses")
	defer flow.finish()
	defer func() {
		if lease != nil && lease.Claimed && !lease.Done && !lease.Sent && !lease.Handoff {
			_ = model.FinishResponseConversation(lease.Conversation.ID, lease.Nonce, "")
		}
	}()
	if _, err := flow.pick(); err != nil {
		if errors.Is(err, model.ErrNoChannel) {
			flow.exhausted()
		} else {
			flow.errorOut(500, "server_error", "internal_error", "failed to check eligible Responses channels")
		}
		return
	}
	pre, err := relay.PreConsumeQuotaAmount(req.userID, req.tokenID, req.tokenUnlimited, relay.EstimateResponsesPreConsumeQuotaWithPriceData(req.priceFor(flow.settleGroup()), parsed))
	if err != nil {
		writePreConsumeError(flow.w, dto.InboundOpenAI, err)
		return
	}
	flow.preQuota = pre
	defer flow.refundUnlessSettled()
	if lease != nil {
		if err := model.ClaimResponseConversation(lease.Conversation, lease.Nonce); err != nil {
			flow.errorOut(409, "invalid_request_error", "conversation_changed", "conversation changed or another turn is active")
			return
		}
		lease.Claimed = true
	}
	flow.runChannels(func(ch *model.Channel) attemptResult {
		adaptor, _ := relay.GetChatAdaptor(ch.Type)
		url, _ := adaptor.ModeURL(ch.BaseURL, channel.RelayModeResponses)
		if parsed.Compact {
			url = strings.TrimRight(url, "/") + "/compact"
		}
		key, index := flow.selectKey(ch)
		if ref, ok := r.Context().Value(common.CtxResponseReference).(*model.ResponseReference); ok {
			var enabled bool
			key, enabled = model.ResponseCredential(ch, ref.CredentialIdentity)
			if !enabled {
				return attemptResult{outcome: attemptUnsupported}
			}
			for i, k := range ch.Keys() {
				if k == key {
					index = i
					break
				}
			}
		}
		capture, err := captureResponseReference(flow, ch, key, parsed)
		if err != nil {
			return attemptResult{outcome: attemptUpstreamError, cause: errors.New("Responses ownership scope could not be captured")}
		}
		if parsed.Compact {
			capture = nil
		}
		var background *model.Task
		if parsed.Background {
			background, err = beginBackgroundResponse(flow, ch, capture, parsed)
			if err != nil {
				flow.errorOut(500, "server_error", "background_snapshot_unavailable", "background billing responsibility could not be persisted")
				return attemptResult{outcome: attemptUncertain}
			}
		}
		if lease != nil {
			lease.Sent = true
		}
		resp, fail := flow.callUpstream(ch, adaptor, upstreamCall{method: http.MethodPost, url: url, contentType: "application/json", body: req.body, key: key, keyIdx: index, sideEffect: true})
		if fail != nil {
			if lease != nil && fail.outcome != attemptUncertain {
				lease.Sent = false
			}
			if background != nil {
				if fail.outcome != attemptUncertain {
					_, _ = service.FailBackgroundSubmission(background, "Provider rejected background submission")
				}
				fail.outcome = attemptUncertain
			}
			return *fail
		}
		defer resp.Body.Close()
		if background != nil {
			return relayBackgroundResponse(flow, resp, background)
		}
		if parsed.Stream {
			return relayResponsesStream(flow, ch, resp, parsed, capture)
		}
		return relayResponsesJSON(flow, ch, resp, parsed, capture)
	})
}

type responseAccounting struct {
	Object string          `json:"object"`
	ID     string          `json:"id"`
	Status string          `json:"status"`
	Output json.RawMessage `json:"output"`
	Store  *bool           `json:"store"`
}

func responseOutcome(status string) (int, bool) {
	switch status {
	case "completed", "incomplete":
		return 1, true
	case "failed":
		return 2, true
	case "cancelled":
		return 3, true
	}
	return 0, false
}

func responsesFallbackUsage(req *dto.ResponsesRequest, output json.RawMessage, text string) *relay.Usage {
	tokens := dto.EstimateTokens(text)
	if len(output) > 0 && string(output) != "null" {
		body, _ := json.Marshal(struct {
			Model string          `json:"model"`
			Input json.RawMessage `json:"input"`
		}{req.Model, output})
		if parsed, err := dto.ParseResponsesRequest(body); err == nil {
			tokens = max(tokens, parsed.InputTokens)
		}
	}
	return &relay.Usage{PromptTokens: req.InputTokens, CompletionTokens: tokens}
}

func settleNativeUsage(flow *relayFlow, ch *model.Channel, usage *relay.Usage, status int) {
	usage.RequestProtocol = flow.req.protocol
	usage.RequestStatus = status
	if err := relay.PostConsumeQuotaWithPrice(flow.req.priceFor(flow.settleGroup()), flow.req.userID, flow.req.tokenID, flow.req.tokenName, flow.req.tokenUnlimited, ch.ID, flow.req.modelName, usage, flow.preQuota, flow.trailString(), time.Since(flow.startTime).Milliseconds(), flow.req.requestedModel); err != nil {
		log.Printf("%s quota settlement failed: %v", flow.mode, err)
	}
	flow.settled = true
}

func relayResponsesJSON(flow *relayFlow, ch *model.Channel, resp *http.Response, req *dto.ResponsesRequest, capture *responseReferenceCapture) attemptResult {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxModeResponseBytes+1))
	if err != nil || len(body) > maxModeResponseBytes {
		return attemptResult{outcome: attemptUncertain, cause: errors.New("Responses body could not be read within the response limit")}
	}
	var result responseAccounting
	if err := json.Unmarshal(body, &result); err != nil || result.ID == "" {
		return attemptResult{outcome: attemptUncertain, cause: errors.New("invalid native Responses object")}
	}
	status, terminal := responseOutcome(result.Status)
	if req.Compact && result.Object == "response.compaction" {
		status, terminal = 1, true
	}
	if !terminal {
		return attemptResult{outcome: attemptUncertain, cause: errors.New("upstream returned a nonterminal foreground Response")}
	}
	usage, usageErr := dto.ParseResponsesUsage(body)
	if usageErr != nil || usage == nil {
		usage = responsesFallbackUsage(req, result.Output, "")
		if status != 1 && usage.CompletionTokens == 0 {
			usage = &relay.Usage{}
		}
	}
	flow.w.Header().Set("X-StarHub-Response-Reference", observeResponseReference(flow, capture, result, usage))
	writePassthroughBody(flow.w, resp, body)
	settleNativeUsage(flow, ch, usage, status)
	outcome := attemptSuccess
	if status != 1 {
		outcome = attemptClientError
	}
	return attemptResult{outcome: outcome, statusCode: resp.StatusCode, usage: usage}
}

type nativeStreamForwarder struct{ w http.ResponseWriter }

func (f nativeStreamForwarder) Write(p []byte) (int, error) {
	n, err := f.w.Write(p)
	if flusher, ok := f.w.(http.Flusher); ok {
		flusher.Flush()
	}
	return n, err
}

func relayResponsesStream(flow *relayFlow, ch *model.Channel, resp *http.Response, req *dto.ResponsesRequest, capture *responseReferenceCapture) attemptResult {
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		return relayResponsesJSON(flow, ch, resp, req, capture)
	}
	flow.w.Header().Add("Trailer", "X-StarHub-Response-Reference")
	flow.w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	flow.w.Header().Set("Cache-Control", "no-cache")
	flow.w.Header().Set("X-Accel-Buffering", "no")
	// Tee raw bytes so comments, IDs, retries, extension events and numeric
	// precision survive. Parsing is an accounting observer, not a serializer.
	reader := channel.NewSSEByteReader(io.TeeReader(io.LimitReader(resp.Body, maxModeResponseBytes+1), nativeStreamForwarder{flow.w}))
	defer reader.Release()
	var usage *relay.Usage
	var output json.RawMessage
	var terminalResponse responseAccounting
	var text strings.Builder
	status, terminal := 2, false
	var streamErr error
	for {
		frame, err := reader.Next()
		if err != nil {
			streamErr = err
			break
		}
		var event struct {
			Type     string             `json:"type"`
			Delta    string             `json:"delta"`
			Response responseAccounting `json:"response"`
		}
		if err := json.Unmarshal(frame.Data, &event); err != nil {
			continue
		}
		switch event.Type {
		case "response.output_text.delta", "response.function_call_arguments.delta", "response.custom_tool_call_input.delta", "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
			text.WriteString(event.Delta)
		}
		if parsed, err := dto.ParseResponsesUsage(frame.Data); err == nil && parsed != nil {
			usage = parsed
		}
		switch event.Type {
		case "response.completed", "response.incomplete", "response.failed", "response.cancelled":
			status, terminal = responseOutcome(event.Response.Status)
			output = append(json.RawMessage(nil), event.Response.Output...)
			terminalResponse = event.Response
		case "error":
			status, terminal = 2, true
		}
		if terminal {
			break
		}
	}
	if usage == nil {
		usage = responsesFallbackUsage(req, output, text.String())
		if status != 1 && usage.CompletionTokens == 0 {
			usage = &relay.Usage{}
		}
	}
	flow.w.Header().Set("X-StarHub-Response-Reference", observeResponseReference(flow, capture, terminalResponse, usage))
	settleNativeUsage(flow, ch, usage, status)
	if !terminal {
		return attemptResult{outcome: attemptUncertain, cause: fmt.Errorf("Responses stream ended before a terminal event: %w", streamErr), usage: usage}
	}
	outcome := attemptSuccess
	if status != 1 {
		outcome = attemptClientError
	}
	return attemptResult{outcome: outcome, statusCode: resp.StatusCode, usage: usage}
}

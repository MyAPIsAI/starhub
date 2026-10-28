package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"starhub/model"
	"starhub/relay"
	"starhub/relay/channel"
	"starhub/relay/dto"
)

// RelayResponses forwards native foreground Responses JSON and SSE while
// retaining the gateway's authorization, reservation and settlement flow.
func RelayResponses(w http.ResponseWriter, r *http.Request) {
	req := relayRequestFromContext(r)
	parsed, err := dto.ParseResponsesRequest(req.body)
	if err == nil {
		err = relay.ValidateResponsesRequest(parsed)
	}
	if err != nil {
		writeModeError(w, 400, "invalid_request_error", "invalid_request_body", err.Error())
		return
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
	flow := newRelayFlow(r.Context(), w, req, dto.InboundOpenAI, "responses")
	defer flow.finish()
	if _, err := flow.pick(); err != nil {
		if errors.Is(err, model.ErrNoChannel) {
			flow.exhausted()
		} else {
			flow.errorOut(500, "server_error", "internal_error", "failed to check eligible Responses channels")
		}
		return
	}
	pre, err := relay.PreConsumeQuotaAmount(req.userID, req.tokenID, req.tokenUnlimited, relay.EstimateResponsesPreConsumeQuota(req.modelName, flow.settleGroup(), req.userGroup, parsed))
	if err != nil {
		writePreConsumeError(flow.w, dto.InboundOpenAI, err)
		return
	}
	flow.preQuota = pre
	defer flow.refundUnlessSettled()
	flow.runChannels(func(ch *model.Channel) attemptResult {
		adaptor, _ := relay.GetChatAdaptor(ch.Type)
		url, _ := adaptor.ModeURL(ch.BaseURL, channel.RelayModeResponses)
		key, index := flow.selectKey(ch)
		resp, fail := flow.callUpstream(ch, adaptor, upstreamCall{method: http.MethodPost, url: url, contentType: "application/json", body: req.body, key: key, keyIdx: index, sideEffect: true})
		if fail != nil {
			return *fail
		}
		defer resp.Body.Close()
		if parsed.Stream {
			return relayResponsesStream(flow, ch, resp, parsed)
		}
		return relayResponsesJSON(flow, ch, resp, parsed)
	})
}

type responseAccounting struct {
	ID     string          `json:"id"`
	Status string          `json:"status"`
	Output json.RawMessage `json:"output"`
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

func settleResponses(flow *relayFlow, ch *model.Channel, usage *relay.Usage, status int) {
	usage.RequestProtocol = flow.req.protocol
	usage.RequestStatus = status
	if err := relay.PostConsumeQuotaWithUserGroup(flow.req.userID, flow.req.tokenID, flow.req.tokenName, flow.req.tokenUnlimited, ch.ID, flow.req.modelName, flow.settleGroup(), flow.req.userGroup, usage, flow.preQuota, flow.trailString(), time.Since(flow.startTime).Milliseconds(), flow.req.requestedModel); err != nil {
		log.Printf("Responses quota settlement failed: %v", err)
	}
	flow.settled = true
}

func relayResponsesJSON(flow *relayFlow, ch *model.Channel, resp *http.Response, req *dto.ResponsesRequest) attemptResult {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxModeResponseBytes+1))
	if err != nil || len(body) > maxModeResponseBytes {
		return attemptResult{outcome: attemptUncertain, cause: errors.New("Responses body could not be read within the response limit")}
	}
	var result responseAccounting
	if err := json.Unmarshal(body, &result); err != nil || result.ID == "" {
		return attemptResult{outcome: attemptUncertain, cause: errors.New("invalid native Responses object")}
	}
	status, terminal := responseOutcome(result.Status)
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
	writePassthroughBody(flow.w, resp, body)
	settleResponses(flow, ch, usage, status)
	outcome := attemptSuccess
	if status != 1 {
		outcome = attemptClientError
	}
	return attemptResult{outcome: outcome, statusCode: resp.StatusCode, usage: usage}
}

type responsesForwarder struct{ w http.ResponseWriter }

func (f responsesForwarder) Write(p []byte) (int, error) {
	n, err := f.w.Write(p)
	if flusher, ok := f.w.(http.Flusher); ok {
		flusher.Flush()
	}
	return n, err
}

func relayResponsesStream(flow *relayFlow, ch *model.Channel, resp *http.Response, req *dto.ResponsesRequest) attemptResult {
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		return relayResponsesJSON(flow, ch, resp, req)
	}
	flow.w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	flow.w.Header().Set("Cache-Control", "no-cache")
	flow.w.Header().Set("X-Accel-Buffering", "no")
	// Tee raw bytes so comments, IDs, retries, extension events and numeric
	// precision survive. Parsing is an accounting observer, not a serializer.
	reader := channel.NewSSEByteReader(io.TeeReader(io.LimitReader(resp.Body, maxModeResponseBytes+1), responsesForwarder{flow.w}))
	defer reader.Release()
	var usage *relay.Usage
	var output json.RawMessage
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
	settleResponses(flow, ch, usage, status)
	if !terminal {
		return attemptResult{outcome: attemptUncertain, cause: fmt.Errorf("Responses stream ended before a terminal event: %w", streamErr), usage: usage}
	}
	outcome := attemptSuccess
	if status != 1 {
		outcome = attemptClientError
	}
	return attemptResult{outcome: outcome, statusCode: resp.StatusCode, usage: usage}
}

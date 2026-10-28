package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"starhub/model"
	"starhub/relay"
	"starhub/relay/channel"
	"starhub/relay/dto"
)

// RelayGemini forwards native model-path requests to Gemini channels only.
func RelayGemini(w http.ResponseWriter, r *http.Request) {
	req := relayRequestFromContext(r)
	parsed, err := dto.ParseGeminiRequest(req.body)
	if err == nil {
		err = relay.ValidateGeminiRequest(parsed)
	}
	if err == nil {
		_, _, err = dto.ParseGeminiPath("/v1beta/models/" + req.modelName + ":generateContent")
	}
	if err != nil {
		dto.WriteRelayError(w, 400, dto.InboundGemini, "invalid_request_error", "invalid_request_body", err.Error())
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
		return err == nil && adaptor.NativeInboundFormat() == dto.InboundGemini, nil
	}
	flow := newRelayFlow(r.Context(), w, req, dto.InboundGemini, "gemini")
	defer flow.finish()
	if _, err := flow.pick(); err != nil {
		if errors.Is(err, model.ErrNoChannel) {
			flow.exhausted()
		} else {
			flow.errorOut(500, "server_error", "internal_error", "failed to check eligible Gemini channels")
		}
		return
	}
	pre, err := relay.PreConsumeQuotaAmount(req.userID, req.tokenID, req.tokenUnlimited, relay.EstimateGeminiPreConsumeQuotaWithPriceData(req.priceFor(flow.settleGroup()), parsed))
	if err != nil {
		writePreConsumeError(flow.w, dto.InboundGemini, err)
		return
	}
	flow.preQuota = pre
	defer flow.refundUnlessSettled()
	flow.runChannels(func(ch *model.Channel) attemptResult {
		adaptor, _ := relay.GetChatAdaptor(ch.Type)
		key, index := flow.selectKey(ch)
		url := adaptor.ChatURL(ch.BaseURL, key, req.modelName, req.stream)
		resp, fail := flow.callUpstream(ch, adaptor, upstreamCall{method: http.MethodPost, url: url, contentType: "application/json", body: req.body, key: key, keyIdx: index, sideEffect: true})
		if fail != nil {
			return *fail
		}
		defer resp.Body.Close()
		if req.stream {
			return relayGeminiStream(flow, ch, resp, parsed)
		}
		return relayGeminiJSON(flow, ch, resp, parsed)
	})
}

type geminiAccounting struct {
	Candidates []struct {
		Content      json.RawMessage `json:"content"`
		FinishReason string          `json:"finishReason"`
	} `json:"candidates"`
	PromptFeedback struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
	Error json.RawMessage `json:"error"`
}

func (result geminiAccounting) outcome() (int, bool) {
	if (len(result.Error) > 0 && string(result.Error) != "null") || (result.PromptFeedback.BlockReason != "" && result.PromptFeedback.BlockReason != "BLOCK_REASON_UNSPECIFIED") {
		return 2, true
	}
	if len(result.Candidates) == 0 || result.Candidates[0].FinishReason == "" || result.Candidates[0].FinishReason == "FINISH_REASON_UNSPECIFIED" {
		return 2, false
	}
	switch result.Candidates[0].FinishReason {
	case "STOP", "MAX_TOKENS":
		return 1, true
	default:
		return 2, true
	}
}

func geminiFallbackOutput(result geminiAccounting) int64 {
	if len(result.Candidates) == 0 || len(result.Candidates[0].Content) == 0 {
		return 0
	}
	// Reuse native part accounting so function arguments and opaque thought
	// signatures are counted while inline image bytes remain excluded.
	body, _ := json.Marshal(struct {
		Contents []json.RawMessage `json:"contents"`
	}{[]json.RawMessage{result.Candidates[0].Content}})
	if parsed, err := dto.ParseGeminiRequest(body); err == nil {
		return parsed.InputTokens
	}
	return 0
}

func relayGeminiJSON(flow *relayFlow, ch *model.Channel, resp *http.Response, req *dto.GeminiRequest) attemptResult {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxModeResponseBytes+1))
	if err != nil || len(body) > maxModeResponseBytes {
		return attemptResult{outcome: attemptUncertain, cause: errors.New("Gemini body could not be read within the response limit")}
	}
	var result geminiAccounting
	if err := json.Unmarshal(body, &result); err != nil {
		return attemptResult{outcome: attemptUncertain, cause: errors.New("invalid native Gemini response")}
	}
	status, terminal := result.outcome()
	if !terminal {
		return attemptResult{outcome: attemptUncertain, cause: errors.New("upstream returned a nonterminal Gemini response")}
	}
	usage, err := dto.ParseGeminiUsage(body)
	if err != nil || usage == nil {
		usage = &relay.Usage{PromptTokens: req.InputTokens, CompletionTokens: geminiFallbackOutput(result)}
	}
	writePassthroughBody(flow.w, resp, body)
	settleNativeUsage(flow, ch, usage, status)
	outcome := attemptSuccess
	if status != 1 {
		outcome = attemptClientError
	}
	return attemptResult{outcome: outcome, statusCode: resp.StatusCode, usage: usage}
}

func relayGeminiStream(flow *relayFlow, ch *model.Channel, resp *http.Response, req *dto.GeminiRequest) attemptResult {
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		return attemptResult{outcome: attemptUncertain, cause: errors.New("upstream did not return native Gemini SSE")}
	}
	flow.w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	flow.w.Header().Set("Cache-Control", "no-cache")
	flow.w.Header().Set("X-Accel-Buffering", "no")
	limited := &io.LimitedReader{R: resp.Body, N: maxModeResponseBytes + 1}
	reader := channel.NewSSEByteReader(io.TeeReader(limited, nativeStreamForwarder{flow.w}))
	defer reader.Release()
	var usage *relay.Usage
	var output int64
	status, terminal := 2, false
	var streamErr error
	for {
		frame, err := reader.Next()
		if err != nil {
			streamErr = err
			break
		}
		var chunk geminiAccounting
		if err := json.Unmarshal(frame.Data, &chunk); err != nil {
			continue
		}
		output += geminiFallbackOutput(chunk)
		if parsed, err := dto.ParseGeminiUsage(frame.Data); err == nil && parsed != nil {
			usage = parsed
		}
		if outcome, end := chunk.outcome(); end {
			if !terminal || status == 1 {
				status = outcome
			}
			terminal = true
		}
	}
	if limited.N == 0 {
		streamErr = errors.New("Gemini stream exceeded the response limit")
	}
	if !errors.Is(streamErr, io.EOF) || !terminal {
		status = 2
	}
	if usage == nil {
		usage = &relay.Usage{PromptTokens: req.InputTokens, CompletionTokens: output}
	}
	settleNativeUsage(flow, ch, usage, status)
	if !terminal || !errors.Is(streamErr, io.EOF) {
		return attemptResult{outcome: attemptUncertain, cause: fmt.Errorf("Gemini stream ended without a complete terminal response: %w", streamErr), usage: usage}
	}
	outcome := attemptSuccess
	if status != 1 {
		outcome = attemptClientError
	}
	return attemptResult{outcome: outcome, statusCode: resp.StatusCode, usage: usage}
}

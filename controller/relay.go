package controller

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"starhub/common"
	"starhub/model"
	"starhub/relay"
	"starhub/relay/channel"
	"starhub/relay/dto"
	"starhub/service"
)

var upstreamHTTPClient = &http.Client{Timeout: 10 * time.Minute}

// attemptOutcome classifies how one upstream attempt ended.
type attemptOutcome int

const (
	// attemptSuccess: the response was relayed to the client and billed.
	attemptSuccess attemptOutcome = iota
	// attemptClientError: 4xx — the upstream response was passed through and
	// the retry loop must stop (the request itself is broken or unauthorized).
	attemptClientError
	// attemptUpstreamError: 5xx, timeout or network error — safe to retry on
	// another channel.
	attemptUpstreamError
	// attemptUnsupported: the channel does not serve this relay mode — skip
	// it and keep looking, without poisoning the error trail.
	attemptUnsupported
)

type attemptResult struct {
	outcome attemptOutcome
	usage   *relay.Usage
	content string
	cause   error
}

// relayRequest is the per-request state every relay mode shares (extracted
// from the middleware chain context).
type relayRequest struct {
	userID         int64
	tokenID        int64
	tokenName      string
	tokenUnlimited bool
	group          string
	modelName      string
	body           []byte
	contentType    string
	stream         bool
}

func relayRequestFromContext(r *http.Request) relayRequest {
	ctx := r.Context()
	req := relayRequest{
		userID:         ctxValueInt64(ctx, common.CtxUserID),
		tokenID:        ctxValueInt64(ctx, common.CtxTokenID),
		tokenName:      ctxValueString(ctx, common.CtxTokenName),
		tokenUnlimited: ctxValueBool(ctx, common.CtxTokenUnlimited),
		group:          ctxValueString(ctx, common.CtxUsingGroup),
		modelName:      ctxValueString(ctx, common.CtxRequestModel),
		contentType:    ctxValueString(ctx, common.CtxRequestContentType),
		stream:         ctxValueBool(ctx, common.CtxRequestStream),
	}
	req.body, _ = ctx.Value(common.CtxRequestBody).([]byte)
	return req
}

// relayFlow drives channel selection and the failover retry loop shared by
// every relay mode. The caller provides the per-mode attempt function; the
// flow owns the trail bookkeeping, the terminal error surface and the
// pre-payment state (refund / settle).
type relayFlow struct {
	ctx         context.Context
	w           http.ResponseWriter
	req         relayRequest
	inbound     dto.InboundFormat // error envelope format; "" defaults to openai
	preQuota    int64
	settled     bool
	tried       map[int64]bool
	trail       []string
	lastErr     error
	unsupported int
}

func newRelayFlow(ctx context.Context, w http.ResponseWriter, req relayRequest, inbound dto.InboundFormat) *relayFlow {
	if inbound == "" {
		inbound = dto.InboundOpenAI
	}
	return &relayFlow{ctx: ctx, w: w, req: req, inbound: inbound, tried: map[int64]bool{}}
}

// errorOut writes a gateway error in the inbound format.
func (f *relayFlow) errorOut(status int, errType, code, message string) {
	dto.WriteRelayError(f.w, status, f.inbound, errType, code, message)
}

// pick returns the next untried channel for the request model.
func (f *relayFlow) pick() (*model.Channel, error) {
	return model.GetSatisfiedChannelExcluding(f.req.group, f.req.modelName, f.tried)
}

// begin marks a channel as attempted BEFORE the attempt runs: the settle
// inside the attempt reads the trail, so the current channel must already
// be on it.
func (f *relayFlow) begin(ch *model.Channel) {
	f.tried[ch.ID] = true
	f.trail = append(f.trail, strconv.FormatInt(ch.ID, 10))
}

// applyOutcome keeps the outcome bookkeeping of a finished attempt.
func (f *relayFlow) applyOutcome(res attemptResult) {
	switch res.outcome {
	case attemptUnsupported:
		f.unsupported++
	case attemptUpstreamError:
		f.lastErr = res.cause
	default:
	}
}

// exhausted writes the terminal error when every candidate failed. When
// every channel was merely unsupported the request itself is invalid for
// the model's channels (400 rather than 502).
func (f *relayFlow) exhausted() {
	if len(f.trail) == 0 {
		f.errorOut(http.StatusServiceUnavailable, "server_error", "no_available_channel",
			"no available channel for model "+f.req.modelName+" in group "+f.req.group)
		return
	}
	if f.unsupported == len(f.trail) {
		f.errorOut(http.StatusBadRequest, "invalid_request_error", "unsupported_mode",
			fmt.Sprintf("no channel for model %s serves this API (channels tried: %s)",
				f.req.modelName, strings.Join(f.trail, "->")))
		return
	}
	f.errorOut(http.StatusBadGateway, "server_error", "upstream_unavailable",
		fmt.Sprintf("all %d channel attempt(s) failed for model %s (channels: %s): %v",
			len(f.trail), f.req.modelName, strings.Join(f.trail, "->"), f.lastErr))
}

func (f *relayFlow) trailString() string { return strings.Join(f.trail, "->") }

// refundUnlessSettled returns the pre-payment when the request never
// reached settlement (failure, 4xx passthrough, panic path). Install it
// with defer right after the pre-charge.
func (f *relayFlow) refundUnlessSettled() {
	if f.settled {
		return
	}
	if err := relay.RefundPreConsumedQuota(f.req.tokenID, f.req.userID, f.req.tokenUnlimited, f.preQuota); err != nil {
		log.Printf("refund after failed relay failed: %v", err)
	} else {
		log.Printf("relay for model %s failed on channels [%s], refunded %d quota",
			f.req.modelName, f.trailString(), f.preQuota)
	}
}

// upstreamCall is one fully described upstream request. The key is
// selected once per attempt by the caller (round-robin channels advance
// their cursor on every selection); contentType is set before the adaptor
// signs/prepares headers so signatures cover it (AWS SigV4 canonical
// headers). ctx, when non-nil, overrides the flow context so a streaming
// mode can detach the upstream from the client's request lifetime (P3.1
// image stream: the gateway must drain the upstream to EOF even after the
// client is gone, so the completed-event count — and thus the bill —
// reflects what the upstream actually delivered).
type upstreamCall struct {
	method      string
	url         string
	contentType string
	body        []byte
	key         string
	keyIdx      int
	ctx         context.Context
}

// callUpstream executes the request and applies the shared error policy:
// 4xx is passed through to the client (with the error envelope converted to
// the inbound format and broken keys auto-disabled), 5xx is retryable. A
// non-nil attemptResult means the attempt is over (nothing else may read
// the response); a nil result hands the open response to the caller, who
// owns closing it.
func (f *relayFlow) callUpstream(ch *model.Channel, adaptor channel.ChatAdaptor, call upstreamCall) (*http.Response, *attemptResult) {
	ctx := f.ctx
	if call.ctx != nil {
		ctx = call.ctx
	}
	req, err := http.NewRequestWithContext(ctx, call.method, call.url, bytes.NewReader(call.body))
	if err != nil {
		return nil, &attemptResult{outcome: attemptUpstreamError,
			cause: fmt.Errorf("channel %d: build request: %v", ch.ID, err)}
	}
	if call.contentType == "" {
		call.contentType = "application/json"
	}
	req.Header.Set("Content-Type", call.contentType)
	req.ContentLength = int64(len(call.body))

	if err := adaptor.PrepareRequest(req, call.key, call.body); err != nil {
		return nil, &attemptResult{outcome: attemptUpstreamError,
			cause: fmt.Errorf("channel %d: prepare request: %v", ch.ID, err)}
	}

	resp, err := upstreamHTTPClient.Do(req)
	if err != nil {
		return nil, &attemptResult{outcome: attemptUpstreamError,
			cause: fmt.Errorf("channel %d unreachable: %v", ch.ID, err)}
	}
	if resp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if resp.StatusCode < http.StatusInternalServerError {
			// Client-class error: terminate, pass the upstream answer through
			// (envelope converted to the inbound format). A broken key must
			// not poison later requests: on multi-key channels only the
			// offending key is disabled — including a throttled one (429) —
			// so the channel survives while other keys remain.
			disableKey := service.ShouldDisableChannel(resp.StatusCode, string(body)) ||
				(ch.IsMultiKey() && resp.StatusCode == http.StatusTooManyRequests)
			if disableKey {
				disableBrokenKey(ch, call.keyIdx, resp.StatusCode)
			}
			dto.WriteUpstreamError(f.w, resp.StatusCode, f.inbound, body, resp.Header.Get("Content-Type"))
			return nil, &attemptResult{outcome: attemptClientError}
		}
		return nil, &attemptResult{outcome: attemptUpstreamError,
			cause: fmt.Errorf("channel %d returned %d", ch.ID, resp.StatusCode)}
	}
	return resp, nil
}

// runChannels iterates candidate channels, running attempt per channel,
// until one terminates the request (success or client error).
func (f *relayFlow) runChannels(attempt func(ch *model.Channel) attemptResult) {
	for {
		ch, err := f.pick()
		if err != nil {
			break
		}
		f.begin(ch)
		res := attempt(ch)
		f.applyOutcome(res)
		switch res.outcome {
		case attemptSuccess, attemptClientError:
			return
		}
	}
	f.exhausted()
}

func ctxValueInt64(ctx context.Context, key any) int64 {
	v, _ := ctx.Value(key).(int64)
	return v
}

func ctxValueString(ctx context.Context, key any) string {
	v, _ := ctx.Value(key).(string)
	return v
}

func ctxValueBool(ctx context.Context, key any) bool {
	v, _ := ctx.Value(key).(bool)
	return v
}

// ---------------------------------------------------------------------------
// Chat relay: any inbound format x any channel (P3.2 format matrix)
// ---------------------------------------------------------------------------

// RelayChat serves POST /v1/chat/completions (OpenAI inbound).
func RelayChat(w http.ResponseWriter, r *http.Request) {
	relayChat(w, r, dto.InboundOpenAI)
}

// RelayClaudeChat serves POST /v1/messages (Claude inbound).
func RelayClaudeChat(w http.ResponseWriter, r *http.Request) {
	relayChat(w, r, dto.InboundClaude)
}

func relayChat(w http.ResponseWriter, r *http.Request, inbound dto.InboundFormat) {
	req := relayRequestFromContext(r)
	if req.modelName == "" {
		dto.WriteRelayError(w, http.StatusInternalServerError, inbound,
			"server_error", "internal_error", "request model is missing")
		return
	}

	// Parse the inbound protocol into IR. Validation happens before any
	// quota moves.
	ir, err := parseInboundChat(req.body, inbound)
	if err != nil {
		dto.WriteRelayError(w, http.StatusBadRequest, inbound,
			"invalid_request_error", "invalid_request_body", "invalid request body: "+err.Error())
		return
	}
	ir.Stream = req.stream

	// Billing multiplier validation (n / max_tokens) must precede the
	// pre-charge: what is validated is exactly what bills.
	if err := validateInboundChat(req.body, ir, inbound); err != nil {
		dto.WriteRelayError(w, http.StatusBadRequest, inbound,
			"invalid_request_error", "invalid_request_body", err.Error())
		return
	}

	// The first selection doubles as the availability check: fail with 503
	// before any quota is touched.
	if _, err := model.GetSatisfiedChannel(req.group, req.modelName); err != nil {
		if errors.Is(err, model.ErrNoChannel) {
			dto.WriteRelayError(w, http.StatusServiceUnavailable, inbound,
				"server_error", "no_available_channel",
				"no available channel for model "+req.modelName+" in group "+req.group)
			return
		}
		dto.WriteRelayError(w, http.StatusInternalServerError, inbound,
			"server_error", "channel_selection_failed", err.Error())
		return
	}

	// Pre-charge: the prompt estimate comes from the IR (protocol
	// independent), the completion budget from the native multiplier fields.
	var preQuota int64
	var preErr error
	if inbound == dto.InboundClaude {
		preQuota, preErr = relay.PreConsumeIRQuota(req.userID, req.tokenID, req.tokenUnlimited,
			req.modelName, req.group, ir)
	} else {
		preQuota, preErr = relay.PreConsumeQuota(req.userID, req.tokenID, req.tokenUnlimited,
			req.modelName, req.group, req.body)
	}
	if preErr != nil {
		writePreConsumeError(w, inbound, preErr)
		return
	}

	flow := newRelayFlow(r.Context(), w, req, inbound)
	flow.preQuota = preQuota
	defer flow.refundUnlessSettled()

	flow.runChannels(func(ch *model.Channel) attemptResult {
		return chatAttempt(flow, ch, ir, inbound)
	})
}

func parseInboundChat(body []byte, inbound dto.InboundFormat) (*dto.ChatRequest, error) {
	if inbound == dto.InboundClaude {
		return dto.ParseClaudeChatRequest(body)
	}
	return dto.ParseOpenAIChatRequest(body)
}

// validateInboundChat applies the billing multiplier bounds per inbound
// protocol: OpenAI bodies carry n / max_tokens / max_completion_tokens;
// Claude bodies only max_tokens.
func validateInboundChat(body []byte, ir *dto.ChatRequest, inbound dto.InboundFormat) error {
	if inbound != dto.InboundClaude {
		return relay.ValidateChatRequest(body)
	}
	if ir.MaxTokens < 0 || ir.MaxTokens > common.MaxChatMaxTokens {
		return fmt.Errorf("max_tokens must be between 0 and %d, got %d", common.MaxChatMaxTokens, ir.MaxTokens)
	}
	return nil
}

func writePreConsumeError(w http.ResponseWriter, inbound dto.InboundFormat, err error) {
	if errors.Is(err, relay.ErrInsufficientTokenQuota) || errors.Is(err, relay.ErrInsufficientUserQuota) {
		dto.WriteRelayError(w, http.StatusForbidden, inbound,
			"insufficient_quota", "insufficient_quota", err.Error())
		return
	}
	dto.WriteRelayError(w, http.StatusInternalServerError, inbound,
		"server_error", "pre_consume_failed", err.Error())
}

// chatAttempt relays one chat attempt through one channel. When the
// channel speaks the client's protocol natively the bodies pass through
// byte-faithfully; otherwise the IR conversion path applies (P3.2 matrix).
func chatAttempt(flow *relayFlow, ch *model.Channel, ir *dto.ChatRequest, inbound dto.InboundFormat) attemptResult {
	adaptor, err := relay.GetChatAdaptor(ch.Type)
	if err != nil {
		return attemptResult{outcome: attemptUpstreamError, cause: fmt.Errorf("channel %d: %v", ch.ID, err)}
	}

	// Build the outbound body: passthrough for native protocols, IR
	// conversion for everything else.
	var outBody []byte
	passthrough := adaptor.NativeInboundFormat() == inbound
	if passthrough {
		outBody, err = adaptor.PreparePassthroughBody(flow.req.body, flow.req.stream)
	} else {
		outBody, err = adaptor.ConvertChatRequest(ir)
	}
	if err != nil {
		return attemptResult{outcome: attemptUpstreamError,
			cause: fmt.Errorf("channel %d: convert request: %v", ch.ID, err)}
	}

	// Select the upstream key once per attempt: the URL of signed
	// providers depends on it and round-robin channels advance their
	// cursor on every selection.
	upstreamKey, keyIdx := ch.SelectKey()
	call := upstreamCall{
		method: http.MethodPost,
		url:    adaptor.ChatURL(ch.BaseURL, upstreamKey, flow.req.modelName, flow.req.stream),
		body:   outBody,
		key:    upstreamKey,
		keyIdx: keyIdx,
	}
	resp, fail := flow.callUpstream(ch, adaptor, call)
	if fail != nil {
		return *fail
	}
	defer resp.Body.Close()

	if passthrough {
		usage, content, herr := adaptor.HandlePassthroughResponse(flow.w, resp, flow.req.stream)
		if herr != nil {
			log.Printf("relay response handling error: %v", herr)
		}
		return settleChatAttempt(flow, ch, ir, usage, content)
	}
	return convertedChatAttempt(flow, ch, adaptor, ir, inbound, resp)
}

// convertedChatAttempt serves a cross-format attempt: the upstream speaks
// a different protocol than the client, so responses flow through IR and
// are re-serialized in the inbound format.
func convertedChatAttempt(flow *relayFlow, ch *model.Channel, adaptor channel.ChatAdaptor,
	ir *dto.ChatRequest, inbound dto.InboundFormat, resp *http.Response) attemptResult {

	if !flow.req.stream {
		chatResp, err := adaptor.ParseChatResponse(resp)
		if err != nil {
			return attemptResult{outcome: attemptUpstreamError,
				cause: fmt.Errorf("channel %d: parse response: %v", ch.ID, err)}
		}
		relay.WriteChatResponse(flow.w, inbound, chatResp)
		return settleChatAttempt(flow, ch, ir, chatResp.Usage, chatResp.Text)
	}

	// Streamed conversion. Some upstreams answer a stream request with a
	// plain JSON body: wrap it into the client's SSE format instead of
	// producing an empty stream (the P3.1 "JSON as SSE" pattern). Any
	// other content type (SSE, the AWS binary event stream) flows through
	// the adaptor's stream parser.
	if common.AssertJSONContentType(resp.Header.Get("Content-Type")) {
		chatResp, err := adaptor.ParseChatResponse(resp)
		if err != nil {
			return attemptResult{outcome: attemptUpstreamError,
				cause: fmt.Errorf("channel %d: parse response: %v", ch.ID, err)}
		}
		emitter := relay.NewChatStreamEmitter(flow.w, inbound, flow.req.modelName)
		if chatResp.Text != "" {
			_ = emitter.Emit(&dto.ChatStreamEvent{TextDelta: chatResp.Text})
		}
		_ = emitter.Finish(chatResp.Usage, chatResp.FinishReason)
		return settleChatAttempt(flow, ch, ir, chatResp.Usage, chatResp.Text)
	}

	reader, err := adaptor.StreamChatEvents(resp)
	if err != nil {
		return attemptResult{outcome: attemptUpstreamError,
			cause: fmt.Errorf("channel %d: open stream: %v", ch.ID, err)}
	}
	defer reader.Close()

	emitter := relay.NewChatStreamEmitter(flow.w, inbound, flow.req.modelName)
	var lastUsage *relay.Usage
	var content strings.Builder
	var finishReason string
	for {
		ev, err := reader.Next()
		if err != nil {
			if err != io.EOF {
				log.Printf("chat stream ended with error: %v", err)
			}
			break
		}
		if ev.Usage != nil && ev.Usage.TotalTokens() > 0 {
			lastUsage = ev.Usage
		}
		if ev.FinishReason != "" {
			finishReason = ev.FinishReason
		}
		if ev.TextDelta != "" {
			if eerr := emitter.Emit(ev); eerr != nil {
				// The client is gone; keep draining the upstream so the
				// usage report still settles the bill.
				continue
			}
			content.WriteString(ev.TextDelta)
		}
	}
	_ = emitter.Finish(lastUsage, finishReason)
	return settleChatAttempt(flow, ch, ir, lastUsage, content.String())
}

// settleChatAttempt balances the books of a successful attempt: missing
// usage falls back to the IR-based estimate so a usage-less upstream still
// bills (P2 invariant).
func settleChatAttempt(flow *relayFlow, ch *model.Channel, ir *dto.ChatRequest, usage *relay.Usage, content string) attemptResult {
	if usage == nil || usage.TotalTokens() == 0 {
		usage = relay.EstimateIRUsage(ir, content)
	}
	if err := relay.PostConsumeQuota(flow.req.userID, flow.req.tokenID, flow.req.tokenName,
		flow.req.tokenUnlimited, ch.ID, flow.req.modelName, flow.req.group, usage,
		flow.preQuota, flow.trailString()); err != nil {
		log.Printf("post consume quota failed: %v", err)
	}
	flow.settled = true
	return attemptResult{outcome: attemptSuccess, usage: usage, content: content}
}

// disableBrokenKey isolates a failing upstream credential. Single-key
// channels are auto-disabled as a whole; multi-key channels lose only the
// offending key and only get disabled once every key is dead.
func disableBrokenKey(ch *model.Channel, keyIdx, statusCode int) {
	if !ch.IsMultiKey() {
		if err := service.DisableChannel(ch.ID, fmt.Sprintf("upstream returned %d", statusCode)); err != nil {
			log.Printf("auto-disable channel %d failed: %v", ch.ID, err)
		}
		return
	}
	allDisabled, err := model.DisableChannelKey(ch.ID, keyIdx)
	if err != nil {
		log.Printf("disable key %d of channel %d failed: %v", keyIdx, ch.ID, err)
		return
	}
	if allDisabled {
		if err := service.DisableChannel(ch.ID,
			fmt.Sprintf("every upstream key failed (last: %d)", statusCode)); err != nil {
			log.Printf("auto-disable channel %d failed: %v", ch.ID, err)
		}
		return
	}
	log.Printf("channel %d: upstream key %d disabled after %d, other keys keep serving",
		ch.ID, keyIdx, statusCode)
}

func ListModels(w http.ResponseWriter, r *http.Request) {
	group, _ := r.Context().Value(common.CtxUsingGroup).(string)
	models, err := model.ListModelsByGroup(group)
	if err != nil {
		common.RelayError(w, http.StatusInternalServerError,
			"server_error", "list_models_failed", err.Error())
		return
	}
	data := make([]map[string]any, 0, len(models))
	for _, m := range models {
		data = append(data, map[string]any{
			"id":       m,
			"object":   "model",
			"created":  0,
			"owned_by": "starhub",
		})
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}

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
	"starhub/metric"
	"starhub/model"
	"starhub/relay"
	"starhub/relay/channel"
	"starhub/relay/dto"
	"starhub/service"
	"starhub/setting"
)

var upstreamHTTPClient = &http.Client{Timeout: 10 * time.Minute}

// statusWriter wraps http.ResponseWriter to capture the response status
// code so the metric layer can classify the request outcome without
// poking at the writer from every error path.
type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func newStatusWriter(w http.ResponseWriter) *statusWriter {
	return &statusWriter{ResponseWriter: w, status: http.StatusOK}
}

func (w *statusWriter) WriteHeader(code int) {
	if !w.wrote {
		w.status = code
		w.wrote = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.status = http.StatusOK
		w.wrote = true
	}
	return w.ResponseWriter.Write(b)
}

// Flush delegates to the underlying writer so SSE streaming keeps its
// real-time push behavior through the wrapper.
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// upstreamRequestID extracts the provider's request-id from the response
// headers so an operator can correlate a slow attempt with the upstream's
// own logs. Different providers use different header names.
func upstreamRequestID(resp *http.Response) string {
	for _, h := range []string{"X-Request-Id", "Request-Id", "X-Amzn-Requestid", "CF-RAY"} {
		if v := resp.Header.Get(h); v != "" {
			return v
		}
	}
	return ""
}

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
	// attemptUncertain: the upstream may have executed the request but the
	// result is indeterminate (e.g. local persistence failed after upstream
	// accepted a side-effecting operation). Must not be retried across
	// channels; affinity is preserved. The attempt function must write the
	// error response before returning this value.
	attemptUncertain
)

type attemptResult struct {
	outcome    attemptOutcome
	usage      *relay.Usage
	content    string
	cause      error
	statusCode int // upstream status code (for 4xx/5xx classification); 0 when none
}

// relayRequest is the per-request state every relay mode shares (extracted
// from the middleware chain context).
type relayRequest struct {
	userID         int64
	tokenID        int64
	tokenName      string
	tokenUnlimited bool
	group          string
	userGroup      string
	autoGroups     []string
	modelName      string
	body           []byte
	contentType    string
	stream         bool
	prefixHash     string // prompt-prefix scope for prefix-cache affinity
}

func relayRequestFromContext(r *http.Request) relayRequest {
	ctx := r.Context()
	req := relayRequest{
		userID:         ctxValueInt64(ctx, common.CtxUserID),
		tokenID:        ctxValueInt64(ctx, common.CtxTokenID),
		tokenName:      ctxValueString(ctx, common.CtxTokenName),
		tokenUnlimited: ctxValueBool(ctx, common.CtxTokenUnlimited),
		group:          ctxValueString(ctx, common.CtxUsingGroup),
		userGroup:      ctxValueString(ctx, common.CtxUserGroup),
		modelName:      ctxValueString(ctx, common.CtxRequestModel),
		contentType:    ctxValueString(ctx, common.CtxRequestContentType),
		stream:         ctxValueBool(ctx, common.CtxRequestStream),
		prefixHash:     ctxValueString(ctx, common.CtxRequestPrefixHash),
	}
	req.autoGroups, _ = ctx.Value(common.CtxAutoGroups).([]string)
	req.body, _ = ctx.Value(common.CtxRequestBody).([]byte)
	return req
}

// billingGroup returns the group used for the pre-charge: the first auto
// group when the using group is "auto" (cheapest-first), otherwise the
// using group itself.
func (req relayRequest) billingGroup() string {
	if req.group == common.AutoGroup && len(req.autoGroups) > 0 {
		return req.autoGroups[0]
	}
	return req.group
}

// relayFlow drives channel selection and the failover retry loop shared by
// every relay mode. The caller provides the per-mode attempt function; the
// flow owns the trail bookkeeping, the terminal error surface and the
// pre-payment state (refund / settle).
type relayFlow struct {
	ctx          context.Context
	w            http.ResponseWriter
	req          relayRequest
	inbound      dto.InboundFormat // error envelope format; "" defaults to openai
	preQuota     int64
	settled      bool
	tried        map[int64]bool
	trail        []string
	lastErr      error
	unsupported  int
	sent         int    // actual upstream send attempts (unsupported skips excluded)
	uncertain    bool   // a side-effecting attempt may have executed upstream
	budgetStop   bool   // stopped because the total attempt budget was exhausted
	currentGroup string // group of the current attempt (for auto-group walk)
	groupIdx     int    // cursor into req.autoGroups (auto-group mode)
	startTime    time.Time // request start for duration_ms logging
	// Observability (P5): the request-level span and the current attempt
	// span. The attempt span is set by runChannels before each attempt
	// and read by callUpstream / the streaming loops to record TTFT.
	span    *metric.RequestSpan
	attempt *metric.AttemptSpan
	sw      *statusWriter
	mode    string // relay mode for the metric label (chat, embeddings, ...)

	// Affinity bookkeeping per request (used by pick and noteAffinity).
	affinityPreferred int64 // channel picked by affinity (0 when none)
	affinityKeyIdx    int   // key index used for the current attempt (-1 = none)
	affinityFailed    bool  // true when the affinity channel failed this request
	affinityCounted   bool  // true when affinity hit/miss has been counted
}

func newRelayFlow(ctx context.Context, w http.ResponseWriter, req relayRequest, inbound dto.InboundFormat, mode string) *relayFlow {
	if inbound == "" {
		inbound = dto.InboundOpenAI
	}
	sw := newStatusWriter(w)
	flow := &relayFlow{
		ctx: ctx, w: sw, sw: sw, req: req, inbound: inbound, mode: mode,
		startTime:      time.Now(),
		tried:          map[int64]bool{},
		affinityKeyIdx: -1,
		span: metric.BeginRequest(metric.RequestMeta{
			UserID:  req.userID,
			TokenID: req.tokenID,
			Model:   req.modelName,
			Group:   req.group,
			Stream:  req.stream,
			Mode:    mode,
		}),
	}
	// Auto-group mode: start walking from the first group. The pre-charge
	// used billingGroup() (the first / cheapest group); the settlement uses
	// currentGroup, which advances when a group is exhausted.
	if req.group == common.AutoGroup && len(req.autoGroups) > 0 {
		flow.currentGroup = req.autoGroups[0]
		flow.groupIdx = 0
	} else {
		flow.currentGroup = req.group
	}
	return flow
}

// finish closes the request-level metric span. Install it with defer
// right after newRelayFlow.
func (f *relayFlow) finish() {
	if f.span == nil {
		return
	}
	status := f.sw.status
	if !f.sw.wrote {
		status = 0 // nothing written — internal error / panic path
	}
	f.span.EndRequest(status)
}

// errorOut writes a gateway error in the inbound format.
func (f *relayFlow) errorOut(status int, errType, code, message string) {
	dto.WriteRelayError(f.w, status, f.inbound, errType, code, message)
}

// pick returns the next untried channel for the request model. In auto-group
// mode, when the current group is exhausted the walk advances to the next
// group in the token's auto-group sequence (cheapest-first, degrade to
// pricier on failure). It also populates affinityPreferred for metrics and
// noteAffinity.
func (f *relayFlow) pick() (*model.Channel, error) {
	for {
		preferred, hashKey := int64(0), ""
		if f.affinityEligible() {
			switch setting.RoutingAffinityMode() {
			case setting.AffinityModeHash:
				hashKey = f.affinityHashKey()
			default:
				id, ok := model.AffinityChannel(f.affinityKey())
				if ok && !f.tried[id] && !model.ChannelCoolingDown(id) {
					preferred = id
				}
			}
		}
		var (
			ch  *model.Channel
			err error
		)
		if hashKey != "" {
			ch, err = model.GetSatisfiedChannelHashed(f.currentGroup, f.req.modelName, f.tried, hashKey)
		} else {
			ch, err = model.GetSatisfiedChannelPreferred(f.currentGroup, f.req.modelName, f.tried, preferred)
		}
		if err == nil {
			f.affinityPreferred = 0
			if preferred != 0 && ch.ID == preferred {
				f.affinityPreferred = ch.ID
			}
			if !f.affinityCounted && f.affinityEligible() && setting.RoutingAffinityMode() != setting.AffinityModeHash {
				f.affinityCounted = true
				if preferred != 0 && ch.ID == preferred {
					metric.RecordAffinity(metric.AffinityHit)
				} else {
					metric.RecordAffinity(metric.AffinityMiss)
				}
			}
			return ch, nil
		}
		if !errors.Is(err, model.ErrNoChannel) {
			return nil, err
		}
		// Current group exhausted: advance to the next auto group, if any.
		if f.req.group != common.AutoGroup || f.groupIdx >= len(f.req.autoGroups)-1 {
			return nil, model.ErrNoChannel
		}
		f.groupIdx++
		f.currentGroup = f.req.autoGroups[f.groupIdx]
	}
}

// affinityKey returns the sticky-affinity scope of the request: the token,
// model, group and optional prefix scope. The group is part of the key so the
// auto-group walk keeps a preference inside its own group.
func (f *relayFlow) affinityKey() model.AffinityKey {
	return model.AffinityKey{
		TokenID: f.req.tokenID,
		Model:   f.req.modelName,
		Group:   f.currentGroup,
		Prefix:  f.affinityPrefix(),
	}
}

// affinityPrefix returns the prompt-prefix scope or "" when prefix-cache
// affinity is disabled, so the key degrades to channel-level stickiness.
func (f *relayFlow) affinityPrefix() string {
	if !setting.RoutingAffinityPrefixCacheEnabled() {
		return ""
	}
	return f.req.prefixHash
}

// affinityHashKey builds the deterministic hash key for rendezvous mode.
func (f *relayFlow) affinityHashKey() string {
	k := f.affinityKey()
	return fmt.Sprintf("%d|%s|%s|%s", k.TokenID, k.Model, k.Group, k.Prefix)
}

// affinityEligible reports whether sticky routing applies to this request.
func (f *relayFlow) affinityEligible() bool {
	return f.req.tokenID > 0 && setting.RoutingAffinityEnabled()
}

// settleGroup returns the group the settlement should bill through: the
// current group of the successful attempt (auto-group aware).
func (f *relayFlow) settleGroup() string {
	return f.currentGroup
}

// begin marks a channel as attempted BEFORE the attempt runs: the settle
// inside the attempt reads the trail, so the current channel must already
// be on it.
func (f *relayFlow) begin(ch *model.Channel) {
	f.tried[ch.ID] = true
	f.trail = append(f.trail, strconv.FormatInt(ch.ID, 10))
}

// selectKey picks the upstream key for one attempt, honoring a remembered
// key-level affinity so prefix-cache traffic stays on the same account. The
// chosen index is recorded for noteAffinity to persist on success.
func (f *relayFlow) selectKey(ch *model.Channel) (string, int) {
	f.affinityKeyIdx = -1
	if f.affinityEligible() {
		if idx, ok := model.AffinityKeyIndex(f.affinityKey()); ok {
			if aid, ok := model.AffinityChannel(f.affinityKey()); ok && aid == ch.ID {
				if key, ok := ch.KeyAt(idx); ok {
					f.affinityKeyIdx = idx
					return key, idx
				}
			}
		}
	}
	key, idx := ch.SelectKey()
	f.affinityKeyIdx = idx
	return key, idx
}

// noteAffinity updates the sticky preference after an attempt. Only a
// successful attempt is remembered; a retryable failure of the remembered
// channel clears it so the next request does not keep hitting a broken
// upstream first. 4xx (client error) is intentionally a no-op.
func (f *relayFlow) noteAffinity(ch *model.Channel, outcome attemptOutcome) {
	if !f.affinityEligible() {
		return
	}
	key := f.affinityKey()
	switch outcome {
	case attemptSuccess:
		keyIdx := -1
		if ch.IsMultiKey() {
			keyIdx = f.affinityKeyIdx
		}
		model.RememberAffinity(key, ch.ID, keyIdx, setting.RoutingAffinityTTL())
		metric.RecordAffinity(metric.AffinityWrite)
		if f.affinityFailed {
			metric.RecordAffinity(metric.AffinityFallbackSuccess)
		}
	case attemptUpstreamError:
		wasPreferred := f.affinityPreferred == ch.ID
		model.ForgetAffinityIf(key, ch.ID)
		metric.RecordAffinity(metric.AffinityClear)
		if wasPreferred {
			f.affinityFailed = true
			metric.RecordAffinity(metric.AffinityPreferredFailed)
			model.CoolDownChannel(ch.ID, setting.RoutingAffinityCooldown())
		}
	}
}

// applyOutcome keeps the outcome bookkeeping of a finished attempt.
func (f *relayFlow) applyOutcome(res attemptResult) {
	switch res.outcome {
	case attemptUnsupported:
		f.unsupported++
	case attemptUpstreamError, attemptUncertain:
		f.lastErr = res.cause
	default:
	}
}

// exhausted writes the terminal error when every candidate failed. When
// every channel was merely unsupported the request itself is invalid for
// the model's channels (400 rather than 502). Once any part of the client
// response has been committed the error envelope must not be appended: the
// attempt is logged and the stream ends without corrupting what was sent.
func (f *relayFlow) exhausted() {
	if f.sw.wrote {
		if f.lastErr != nil {
			log.Printf("relay stopped after the response was committed: %v", f.lastErr)
		}
		return
	}
	if len(f.trail) == 0 {
		groups := f.req.group
		if f.req.group == common.AutoGroup {
			groups = strings.Join(f.req.autoGroups, ",")
		}
		f.errorOut(http.StatusServiceUnavailable, "server_error", "no_available_channel",
			"no available channel for model "+f.req.modelName+" in group "+groups)
		return
	}
	if f.unsupported == len(f.trail) {
		f.errorOut(http.StatusBadRequest, "invalid_request_error", "unsupported_mode",
			fmt.Sprintf("no channel for model %s serves this API (channels tried: %s)",
				f.req.modelName, strings.Join(f.trail, "->")))
		return
	}
	if f.budgetStop {
		f.errorOut(http.StatusBadGateway, "server_error", "retry_budget_exhausted",
			fmt.Sprintf("retry budget exhausted after %d upstream send(s) for model %s (channels: %s): %v",
				f.sent, f.req.modelName, strings.Join(f.trail, "->"), f.lastErr))
		return
	}
	f.errorOut(http.StatusBadGateway, "server_error", "upstream_unavailable",
		fmt.Sprintf("all %d channel attempt(s) failed for model %s (channels: %s): %v",
			len(f.trail), f.req.modelName, strings.Join(f.trail, "->"), f.lastErr))
}

func (f *relayFlow) trailString() string { return strings.Join(f.trail, "->") }

// refundUnlessSettled returns the pre-payment when the request never
// reached settlement (failure, 4xx passthrough, panic path). Install it
// with defer right after the pre-charge. It also logs a failed-request
// entry (Status=2, Quota=0) so the service-quality and error-rate
// analytics reflect real failures.
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
	// Log the failed request for analytics (does not affect quota).
	// Only when a channel was actually tried (trail is non-empty),
	// so pre-channel failures (no channel available) don't inflate
	// usage counts.
	if len(f.trail) > 0 {
		_ = model.InsertFailedLog(&model.Log{
			UserID:       f.req.userID,
			TokenID:      f.req.tokenID,
			TokenName:    f.req.tokenName,
			Model:        f.req.modelName,
			UsedChannels:  f.trailString(),
			DurationMs:   time.Since(f.startTime).Milliseconds(),
		})
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
	// sideEffect marks a call whose repeated execution is unsafe (task
	// submit, image generation). A transport error after the request was
	// sent cannot prove the upstream did not execute it, so such a call is
	// classified as attemptUncertain and is never auto-retried across
	// channels.
	sideEffect bool
	ctx        context.Context
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
		if call.sideEffect {
			return nil, &attemptResult{outcome: attemptUncertain,
				cause: fmt.Errorf("channel %d: send result uncertain: %v", ch.ID, err)}
		}
		return nil, &attemptResult{outcome: attemptUpstreamError,
			cause: fmt.Errorf("channel %d unreachable: %v", ch.ID, err)}
	}
	// Record TTFT (time from attempt start to response headers) and the
	// upstream request-id so a slow channel is identifiable end-to-end.
	if f.attempt != nil {
		f.attempt.RecordHeaders(upstreamRequestID(resp))
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
			return nil, &attemptResult{outcome: attemptClientError, statusCode: resp.StatusCode}
		}
		return nil, &attemptResult{outcome: attemptUpstreamError, statusCode: resp.StatusCode,
			cause: fmt.Errorf("channel %d returned %d", ch.ID, resp.StatusCode)}
	}
	return resp, nil
}

// runChannels iterates candidate channels, running attempt per channel,
// until one terminates the request (success or client error). The loop
// enforces the total attempt budget (RetryTimes+1), a client-cancel guard
// and a response-committed safety boundary. Unsupported skips do not
// consume the attempt budget.
func (f *relayFlow) runChannels(attempt func(ch *model.Channel) attemptResult) {
loop:
	for {
		if f.ctx.Err() != nil || f.sw.wrote {
			break
		}
		ch, err := f.pick()
		if err != nil {
			break
		}
		f.begin(ch)
		f.attempt = f.span.BeginAttempt(ch.ID)
		res := attempt(ch)
		f.span.EndAttempt(f.attempt, attemptOutcomeToMetric(res.outcome), res.statusCode)
		f.attempt = nil
		f.applyOutcome(res)
		f.noteAffinity(ch, res.outcome)
		switch res.outcome {
		case attemptSuccess, attemptClientError:
			return
		case attemptUncertain:
			f.uncertain = true
			if !f.sw.wrote {
				f.errorOut(http.StatusBadGateway, "server_error", "upstream_result_uncertain",
					fmt.Sprintf("upstream result for model %s is uncertain (the request may have executed): %v",
						f.req.modelName, res.cause))
			}
			return
		case attemptUpstreamError:
			f.sent++
			if f.sent > common.RetryTimes {
				f.budgetStop = true
				if f.lastErr == nil {
					f.lastErr = fmt.Errorf("retry budget exhausted after %d upstream send(s)", f.sent)
				}
				break loop
			}
		}
	}
	f.exhausted()
}

// attemptOutcomeToMetric maps the internal attempt outcome to the metric
// package's classification.
func attemptOutcomeToMetric(o attemptOutcome) metric.AttemptClass {
	switch o {
	case attemptSuccess:
		return metric.AttemptSuccess
	case attemptClientError:
		return metric.AttemptClientError
	case attemptUnsupported:
		return metric.AttemptUnsupported
	case attemptUncertain:
		return metric.AttemptUncertain
	default:
		return metric.AttemptUpstreamError
	}
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
	billingGroup := req.billingGroup()
	if _, err := model.GetSatisfiedChannel(billingGroup, req.modelName); err != nil {
		if errors.Is(err, model.ErrNoChannel) {
			dto.WriteRelayError(w, http.StatusServiceUnavailable, inbound,
				"server_error", "no_available_channel",
				"no available channel for model "+req.modelName+" in group "+billingGroup)
			return
		}
		dto.WriteRelayError(w, http.StatusInternalServerError, inbound,
			"server_error", "channel_selection_failed", err.Error())
		return
	}

	// Pre-charge: the prompt estimate comes from the IR (protocol
	// independent), the completion budget from the native multiplier fields.
	// The pre-charge uses the first auto group (cheapest); the settlement
	// uses the actual serving group.
	var preQuota int64
	var preErr error
	if inbound == dto.InboundClaude {
		preQuota, preErr = relay.PreConsumeIRQuotaWithUserGroup(req.userID, req.tokenID, req.tokenUnlimited,
			req.modelName, billingGroup, req.userGroup, ir)
	} else {
		preQuota, preErr = relay.PreConsumeQuotaWithUserGroup(req.userID, req.tokenID, req.tokenUnlimited,
			req.modelName, billingGroup, req.userGroup, req.body)
	}
	if preErr != nil {
		writePreConsumeError(w, inbound, preErr)
		return
	}

	flow := newRelayFlow(r.Context(), w, req, inbound, "chat")
	flow.preQuota = preQuota
	defer flow.refundUnlessSettled()
	defer flow.finish()

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
	upstreamKey, keyIdx := flow.selectKey(ch)
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
			if flow.attempt != nil {
				flow.attempt.RecordFirstToken()
			}
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
	clientGone := false
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
			if flow.attempt != nil {
				flow.attempt.RecordFirstToken()
			}
			if eerr := emitter.Emit(ev); eerr != nil {
				// The client is gone; keep draining the upstream so the
				// usage report still settles the bill.
				clientGone = true
				continue
			}
			content.WriteString(ev.TextDelta)
		}
	}
	_ = emitter.Finish(lastUsage, finishReason)
	if clientGone {
		flow.span.MarkStreamInterrupted()
	}
	return settleChatAttempt(flow, ch, ir, lastUsage, content.String())
}

// settleChatAttempt balances the books of a successful attempt: missing
// usage falls back to the IR-based estimate so a usage-less upstream still
// bills (P2 invariant).
func settleChatAttempt(flow *relayFlow, ch *model.Channel, ir *dto.ChatRequest, usage *relay.Usage, content string) attemptResult {
	if usage == nil || usage.TotalTokens() == 0 {
		usage = relay.EstimateIRUsage(ir, content)
	}
	if err := relay.PostConsumeQuotaWithUserGroup(flow.req.userID, flow.req.tokenID, flow.req.tokenName,
		flow.req.tokenUnlimited, ch.ID, flow.req.modelName, flow.settleGroup(), flow.req.userGroup, usage,
		flow.preQuota, flow.trailString(), time.Since(flow.startTime).Milliseconds()); err != nil {
		log.Printf("post consume quota failed: %v", err)
	}
	flow.settled = true
	return attemptResult{outcome: attemptSuccess, statusCode: http.StatusOK, usage: usage, content: content}
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

package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"starhub/common"
	"starhub/internal/traffic"
	"starhub/internal/upstream"
	"starhub/metric"
	"starhub/model"
	"starhub/relay/dto"
	"starhub/setting"
)

// Realtime relay (P3): a websocket pipe between the client and an upstream
// realtime channel. Frames flow both ways verbatim; text frames from the
// upstream are scanned for the terminal response.done event whose usage
// settles the session bill.

// realtimeIdleTimeout bounds a silent session; audio sessions are chatty
// so this is a liveness floor, not a length limit.
const realtimeIdleTimeout = 10 * time.Minute

// RelayRealtime serves GET /v1/realtime?model=... (websocket upgrade).
func RelayRealtime(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, _ := ctx.Value(common.CtxUserID).(int64)
	tokenID, _ := ctx.Value(common.CtxTokenID).(int64)
	tokenName, _ := ctx.Value(common.CtxTokenName).(string)
	tokenUnlimited, _ := ctx.Value(common.CtxTokenUnlimited).(bool)
	group, _ := ctx.Value(common.CtxUsingGroup).(string)
	userGroup, _ := ctx.Value(common.CtxUserGroup).(string)

	modelName := r.URL.Query().Get("model")
	if modelName == "" {
		common.RelayError(w, http.StatusBadRequest, "invalid_request_error", "model_required",
			"the \"model\" query parameter is required")
		return
	}

	// Pre-charge: per-call price when configured, otherwise the
	// conservative token budget (the session's actual usage settles it).
	// Use the first auto group (cheapest) when the using group is "auto".
	billingGroup := group
	autoGroups, _ := ctx.Value(common.CtxAutoGroups).([]string)
	if group == common.AutoGroup && len(autoGroups) > 0 {
		billingGroup = autoGroups[0]
	}
	pd := NewPriceDataWithUserGroup(modelName, billingGroup, userGroup)
	pd.AmountUnit = common.MoneyUnitNano
	preUsage := &Usage{}
	if !pd.IsPerCall() && !pd.IsExpression() {
		preUsage.PromptTokens = 1
		preUsage.CompletionTokens = common.DefaultCompletionBudgetTokens
	}
	preQuota := ComputeUsageQuota(pd, preUsage)
	admissionID := common.NewNanoReservationID()
	pd.QuotaReservationID = admissionID
	pre, err := PreConsumeQuotaAmount(userID, tokenID, tokenUnlimited, preQuota, admissionID)
	if err != nil {
		writeRealtimePreConsumeError(w, err)
		return
	}

	settled := false
	defer func() {
		if settled {
			return
		}
		if err := RefundPreConsumedQuota(tokenID, userID, tokenUnlimited, pre, admissionID); err != nil {
			log.Printf("refund after failed realtime session failed: %v", err)
		}
	}()

	// Dial an upstream realtime channel first: a failure keeps the client
	// handshake clean (plain HTTP error, no orphaned upgrade). A remembered
	// affinity preference lets a reconnect land on the same upstream so an
	// established session's cache keeps helping.
	tried := map[int64]bool{}
	stageSkipped := map[int64]bool{}
	routingStage := model.RoutingNormal
	trafficRule := traffic.Snapshot().Lookup(group, modelName)
	affinityKey := model.AffinityKey{TokenID: tokenID, Model: modelName, Group: group}
	affinityOn := tokenID > 0 && setting.RoutingAffinityEnabled() &&
		setting.RoutingAffinityMode() != setting.AffinityModeHash
	preferred := int64(0)
	if affinityOn {
		if id, ok := model.AffinityChannel(affinityKey); ok && !model.ChannelCoolingDown(id) {
			preferred = id
		}
	}
	var upstreamConn *WSConn
	var upstreamChannel *model.Channel
	span := metric.BeginRequest(metric.RequestMeta{Model: modelName, Group: group, Mode: "realtime"})
	status := 502
	defer func() { span.EndRequest(status) }()
	var servingAttempt *metric.AttemptSpan
	var servingSession *realtimeProtectionSession
	var servingPolicy upstream.Policy
	policy := upstream.CurrentPolicy()
	started := time.Now()
	sent := 0
	canRetry := true
	for {
		if ctx.Err() != nil || !canRetry || sent >= policy.MaxAttempts || sent > 0 && time.Since(started) >= time.Duration(policy.RetryWindow)*time.Second {
			break
		}
		excluded := make(map[int64]bool, len(tried)+len(stageSkipped))
		for id := range tried {
			excluded[id] = true
		}
		for id := range stageSkipped {
			excluded[id] = true
		}
		var ch *model.Channel
		var err error
		if setting.IsCommercialMode() || routingStage == model.RoutingOrdinaryFallback {
			ch, err = model.GetScoredChannelStage(routingStage, group, modelName, excluded, preferred, "", false, nil, upstream.Profile{Capability: "realtime"})
		} else {
			ch, err = model.GetSatisfiedChannelTraffic(group, modelName, excluded, preferred, "", trafficRule, upstream.Profile{Capability: "realtime"})
		}
		if err != nil {
			if errors.Is(err, model.ErrNoChannel) && routingStage < model.RoutingOrdinaryFallback {
				routingStage++
				if !setting.IsCommercialMode() {
					routingStage = model.RoutingOrdinaryFallback
				}
				stageSkipped = map[int64]bool{}
				continue
			}
			break
		}
		tried[ch.ID] = true
		resourcePolicy, managed, policyErr := model.ReadResourceGatewayPolicy(r.Context(), ch.ID, group, modelName)
		if policyErr != nil {
			break
		}
		if managed && !resourcePolicy.Allows("/v1/realtime", nil, 0, 0) {
			continue
		}
		adaptor, err := GetChatAdaptor(ch.Type)
		if err != nil {
			continue
		}
		actualModel := modelName
		if ch.RuntimeModel != "" {
			actualModel = ch.RuntimeModel
		}
		wsURL, err := adaptor.RealtimeURL(ch.BaseURL, actualModel)
		if err != nil {
			continue // channel type without realtime support
		}
		key, keyIdx := ch.SelectKey()
		if affinityOn && ch.ID == preferred {
			if idx, ok := model.AffinityKeyIndex(affinityKey); ok {
				if k, ok := ch.KeyAt(idx); ok {
					key, keyIdx = k, idx
				}
			}
		}
		key, keyIdx = ch.ProtectedKey(key, keyIdx)
		// Fallback switch and interval take effect per dial, like the HTTP path.
		sendPolicy := upstream.CurrentPolicy()
		protected, err := model.CaptureChannelAttempt(ctx, ch, group, modelName, key, upstream.Profile{Capability: "realtime"}, sendPolicy, func(current *model.Channel, group string) (bool, error) {
			resource, managed, err := model.ReadResourceGatewayPolicy(ctx, current.ID, group, modelName)
			return !managed || resource.Allows("/v1/realtime", nil, 0, 0), err
		})
		if errors.Is(err, upstream.ErrBlocked) {
			delete(tried, ch.ID)
			stageSkipped[ch.ID] = true
			continue
		}
		if err != nil {
			break
		}
		if ch.RuntimeRoute != nil {
			if ch.RuntimeRoute.AttemptID == "" {
				model.BeginRoutingAttempt(ch.RuntimeRoute)
			}
			ch.RuntimeRoute.Sent = true
		}
		header := http.Header{}
		header.Set("Authorization", "Bearer "+key)
		header.Set("OpenAI-Beta", "realtime=v1")
		a := span.BeginAttempt(ch.ID)
		affinity := "disabled"
		if affinityOn {
			affinity = "miss"
			if ch.ID == preferred {
				affinity = "hit"
			}
		}
		a.SetRouteMetadata(metric.AttemptMetadata{ResourceKey: fmt.Sprintf("%d:%s", ch.ID, modelName), Generation: model.ChannelEndpointGeneration(ch), Protocol: "OpenAI", Capability: "realtime", Group: group, Affinity: affinity})
		a.MarkSent()
		conn, err := WSDialContext(ctx, wsURL, header)
		sent++
		verdict := upstream.Verdict{Class: "handshake_success"}
		httpStatus := http.StatusSwitchingProtocols
		if err != nil {
			httpStatus = 0
			verdict = upstream.Verdict{Class: "transport_failure", Scope: "interface", Retry: true}
			var handshake *WSHandshakeError
			if errors.As(err, &handshake) {
				httpStatus = handshake.Status
				verdict = upstream.HTTP(handshake.Status, handshake.Body, handshake.RetryAfter, policy)
			}
			if ctx.Err() != nil {
				verdict = upstream.Verdict{Class: "client_disconnected"}
			}
		}
		retry, reason := upstream.RetryReason(policy, verdict, upstream.RetryInput{Cancelled: ctx.Err() != nil, Attempts: sent, Elapsed: time.Since(started)})
		if err == nil {
			retry, reason = false, "handshake_completed"
		}
		canRetry = retry
		saveCtx, saveCancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		requestID, _ := ctx.Value(common.CtxRequestID).(string)
		recordErr := upstream.Record(model.DB.WithContext(saveCtx), protected, verdict, upstream.Event{RequestID: requestID, Status: httpStatus, Sent: true, Retry: retry, Decision: reason}, policy)
		saveCancel()
		if recordErr != nil {
			log.Printf("realtime protection persistence failed: %v", recordErr)
		}
		if err != nil {
			finishRealtimeRouting(ch.RuntimeRoute, requestID, verdict, httpStatus)
			span.EndAttempt(a, metric.AttemptUpstreamError, 0)
			log.Printf("realtime dial channel %d failed: %v", ch.ID, err)
			if affinityOn && ch.ID == preferred {
				model.ForgetAffinityIf(affinityKey, ch.ID)
				model.CoolDownChannel(ch.ID, setting.RoutingAffinityCooldown())
			}
			continue
		}
		if affinityOn && ch.RuntimeStage != model.RoutingOrdinaryFallback {
			model.RememberAffinity(affinityKey, ch.ID, keyIdx, setting.RoutingAffinityTTL())
		}
		upstreamConn = conn
		upstreamChannel = ch
		servingAttempt = a
		servingSession = newRealtimeProtectionSession(protected)
		servingPolicy = sendPolicy
		break
	}
	if upstreamConn == nil {
		common.RelayError(w, http.StatusBadGateway, "server_error", "upstream_unavailable",
			fmt.Sprintf("no realtime channel for model %s (group %s)", modelName, group))
		return
	}
	defer upstreamConn.Close()
	requestID, _ := ctx.Value(common.CtxRequestID).(string)
	finalVerdict, finalStatus := upstream.Verdict{Class: "unknown"}, 0
	defer func() {
		if err := model.EndRoutingAttempt(upstreamChannel.RuntimeRoute, requestID, routingResultClass(finalVerdict), finalStatus, time.Now()); err != nil {
			log.Printf("realtime routing event failed: %v", err)
		}
	}()

	// Upgrade the client.
	client, err := WSAccept(w, r)
	if err != nil {
		span.EndAttempt(servingAttempt, metric.AttemptClientError, 400)
		// No upgrade happened: the client still sees an HTTP error.
		common.RelayError(w, http.StatusBadRequest, "invalid_request_error", "invalid_upgrade",
			"websocket upgrade required: "+err.Error())
		return
	}
	defer client.Close()

	usage, outputText := pipeRealtime(client, upstreamConn, func(data []byte) {
		verdict, status, terminal := realtimeBusinessResult(data, policy)
		if !terminal {
			return
		}
		finalVerdict, finalStatus = verdict, status
		recordRealtimeBusiness(ctx, servingSession, upstreamChannel.RuntimeRoute, requestID, verdict, status, servingPolicy)
	})
	if usage != nil {
		servingAttempt.RecordUsage(usage.PromptTokens, usage.CompletionTokens)
	}
	span.EndAttempt(servingAttempt, metric.AttemptSuccess, 200)
	status = 101

	// Settle: the response.done usage when the session reported one,
	// otherwise the observed text estimate (audio usage is unavailable on
	// non-reporting upstreams; per-call priced models are unaffected).
	actualUsage := usage
	if actualUsage == nil {
		actualUsage = &Usage{
			PromptTokens:     1,
			CompletionTokens: EstimateTokens(outputText),
		}
	}
	actualUsage.RequestProtocol = "OpenAI"
	actualUsage.RoutingFallback = sent > 1 || upstreamChannel.RuntimeStage != model.RoutingNormal
	if err := PostConsumeQuotaWithPrice(pd, userID, tokenID, tokenName, tokenUnlimited,
		upstreamChannel.ID, modelName, actualUsage, pre,
		fmt.Sprintf("%d", upstreamChannel.ID), 0, ""); err != nil {
		log.Printf("post consume realtime quota failed: %v", err)
	}
	settled = true
}

func writeRealtimePreConsumeError(w http.ResponseWriter, err error) {
	if err == ErrInsufficientTokenQuota || err == ErrInsufficientUserQuota {
		common.RelayError(w, http.StatusForbidden, "insufficient_quota", "insufficient_quota", err.Error())
		return
	}
	common.RelayError(w, http.StatusInternalServerError, "server_error", "pre_consume_failed", err.Error())
}

// realtimeUsage is the usage block of a response.done event (OpenAI
// realtime shape).
type realtimeUsage struct {
	InputTokens       int64 `json:"input_tokens"`
	OutputTokens      int64 `json:"output_tokens"`
	InputTokenDetails *struct {
		CachedTokens int64 `json:"cached_tokens"`
	} `json:"input_token_details"`
	OutputTokenDetails *struct {
		AudioTokens int64 `json:"audio_tokens"`
	} `json:"output_token_details"`
}

// pipeRealtime relays frames both directions until either side closes or
// errors. Pings are answered locally; close frames propagate. Text frames
// from the upstream feed the usage observer.
func pipeRealtime(client, upstream *WSConn, observers ...func([]byte)) (usage *Usage, outputText string) {
	done := make(chan struct{})
	var once sync.Once
	shutdown := func() {
		once.Do(func() { close(done) })
	}

	// client -> upstream
	go func() {
		defer shutdown()
		for {
			client.SetReadDeadline(realtimeIdleTimeout)
			frame, err := client.ReadFrame()
			if err != nil {
				upstream.WriteClose()
				return
			}
			switch frame.Opcode {
			case wsOpPing:
				_ = client.WriteFrame(wsOpPong, frame.Data)
			case wsOpClose:
				upstream.WriteClose()
				return
			default:
				if err := upstream.WriteFrame(frame.Opcode, frame.Data); err != nil {
					return
				}
			}
		}
	}()

	// upstream -> client (with usage observation)
	var text strings.Builder
	var sessionUsage *Usage
	clientGone := false
	func() {
		defer shutdown()
		for {
			upstream.SetReadDeadline(realtimeIdleTimeout)
			frame, err := upstream.ReadFrame()
			if err != nil {
				client.WriteClose()
				return
			}
			switch frame.Opcode {
			case wsOpPing:
				_ = upstream.WriteFrame(wsOpPong, frame.Data)
			case wsOpClose:
				client.WriteClose()
				return
			default:
				if frame.Opcode == wsOpText {
					for _, observe := range observers {
						observe(frame.Data)
					}
					if ev, textDelta := observeRealtimeFrame(frame.Data); ev != nil {
						sessionUsage = ev
					} else if textDelta != "" {
						text.WriteString(textDelta)
					}
				}
				if !clientGone {
					if err := client.WriteFrame(frame.Opcode, frame.Data); err != nil {
						// The client hung up; keep draining the upstream so
						// the usage report still settles the session.
						clientGone = true
					}
				}
			}
		}
	}()

	<-done
	client.Close()
	upstream.Close()
	return sessionUsage, text.String()
}

// observeRealtimeFrame extracts the usage of a response.done event and the
// text of output_text deltas (the fallback billing inputs).
func observeRealtimeFrame(data []byte) (usage *Usage, textDelta string) {
	var ev struct {
		Type     string `json:"type"`
		Delta    string `json:"delta"`
		Response *struct {
			Usage *realtimeUsage `json:"usage"`
		} `json:"response"`
	}
	if err := json.Unmarshal(data, &ev); err != nil {
		return nil, ""
	}
	if ev.Type == "response.done" && ev.Response != nil && ev.Response.Usage != nil {
		u := ev.Response.Usage
		usage = &Usage{
			PromptTokens:     u.InputTokens,
			CompletionTokens: u.OutputTokens,
		}
		if u.InputTokenDetails != nil && u.InputTokenDetails.CachedTokens > 0 {
			usage.PromptTokensDetails = &dto.PromptTokensDetails{
				CachedTokens: u.InputTokenDetails.CachedTokens,
			}
		}
		if u.OutputTokenDetails != nil && u.OutputTokenDetails.AudioTokens > 0 {
			usage.CompletionTokensDetails = &dto.CompletionTokensDetails{
				AudioTokens: u.OutputTokenDetails.AudioTokens,
			}
		}
		return usage, ""
	}
	if ev.Type == "response.output_text.delta" {
		return nil, ev.Delta
	}
	return nil, ""
}

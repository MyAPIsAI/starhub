package relay

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"starhub/common"
	"starhub/model"
	"starhub/relay/dto"
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

	modelName := r.URL.Query().Get("model")
	if modelName == "" {
		common.RelayError(w, http.StatusBadRequest, "invalid_request_error", "model_required",
			"the \"model\" query parameter is required")
		return
	}

	// Pre-charge: per-call price when configured, otherwise the
	// conservative token budget (the session's actual usage settles it).
	pd := NewPriceData(modelName, group)
	preUsage := &Usage{}
	if !pd.IsPerCall() && !pd.IsExpression() {
		preUsage.PromptTokens = 1
		preUsage.CompletionTokens = common.DefaultCompletionBudgetTokens
	}
	preQuota := ComputeUsageQuota(pd, preUsage)
	pre, err := PreConsumeQuotaAmount(userID, tokenID, tokenUnlimited, preQuota)
	if err != nil {
		writeRealtimePreConsumeError(w, err)
		return
	}

	settled := false
	defer func() {
		if settled {
			return
		}
		if err := RefundPreConsumedQuota(tokenID, userID, tokenUnlimited, pre); err != nil {
			log.Printf("refund after failed realtime session failed: %v", err)
		}
	}()

	// Dial an upstream realtime channel first: a failure keeps the client
	// handshake clean (plain HTTP error, no orphaned upgrade).
	tried := map[int64]bool{}
	var upstream *WSConn
	var upstreamChannel *model.Channel
	for {
		ch, err := model.GetSatisfiedChannelExcluding(group, modelName, tried)
		if err != nil {
			break
		}
		tried[ch.ID] = true
		adaptor, err := GetChatAdaptor(ch.Type)
		if err != nil {
			continue
		}
		wsURL, err := adaptor.RealtimeURL(ch.BaseURL, modelName)
		if err != nil {
			continue // channel type without realtime support
		}
		key, _ := ch.SelectKey()
		header := http.Header{}
		header.Set("Authorization", "Bearer "+key)
		header.Set("OpenAI-Beta", "realtime=v1")
		conn, err := WSDial(wsURL, header)
		if err != nil {
			log.Printf("realtime dial channel %d failed: %v", ch.ID, err)
			continue
		}
		upstream = conn
		upstreamChannel = ch
		break
	}
	if upstream == nil {
		common.RelayError(w, http.StatusBadGateway, "server_error", "upstream_unavailable",
			fmt.Sprintf("no realtime channel for model %s (group %s)", modelName, group))
		return
	}
	defer upstream.Close()

	// Upgrade the client.
	client, err := WSAccept(w, r)
	if err != nil {
		// No upgrade happened: the client still sees an HTTP error.
		common.RelayError(w, http.StatusBadRequest, "invalid_request_error", "invalid_upgrade",
			"websocket upgrade required: "+err.Error())
		return
	}
	defer client.Close()

	usage, outputText := pipeRealtime(client, upstream)

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
	if err := PostConsumeQuota(userID, tokenID, tokenName, tokenUnlimited,
		upstreamChannel.ID, modelName, group, actualUsage, pre,
		fmt.Sprintf("%d", upstreamChannel.ID)); err != nil {
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
func pipeRealtime(client, upstream *WSConn) (usage *Usage, outputText string) {
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

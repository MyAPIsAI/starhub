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
)

type attemptResult struct {
	outcome attemptOutcome
	usage   *relay.Usage
	content string
	cause   error
}

func Relay(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	modelName, _ := ctx.Value(common.CtxRequestModel).(string)
	body, _ := ctx.Value(common.CtxRequestBody).([]byte)
	stream, _ := ctx.Value(common.CtxRequestStream).(bool)
	group, _ := ctx.Value(common.CtxUsingGroup).(string)
	userID, _ := ctx.Value(common.CtxUserID).(int64)
	tokenID, _ := ctx.Value(common.CtxTokenID).(int64)
	tokenName, _ := ctx.Value(common.CtxTokenName).(string)
	tokenUnlimited, _ := ctx.Value(common.CtxTokenUnlimited).(bool)

	if modelName == "" {
		common.RelayError(w, http.StatusInternalServerError,
			"server_error", "internal_error", "request model is missing")
		return
	}

	// The first selection doubles as the availability check: fail with 503
	// before any quota is touched.
	ch, err := model.GetSatisfiedChannel(group, modelName)
	if err != nil {
		if errors.Is(err, model.ErrNoChannel) {
			common.RelayError(w, http.StatusServiceUnavailable,
				"server_error", "no_available_channel",
				"no available channel for model "+modelName+" in group "+group)
			return
		}
		common.RelayError(w, http.StatusInternalServerError,
			"server_error", "channel_selection_failed", err.Error())
		return
	}

	// Convert with the first channel's adaptor; the request body validation
	// must also happen before money moves.
	adaptor, err := relay.GetAdaptor(ch.Type)
	if err != nil {
		common.RelayError(w, http.StatusInternalServerError,
			"server_error", "unsupported_channel_type", err.Error())
		return
	}
	outBody, err := adaptor.ConvertRequestBody(body, stream)
	if err != nil {
		common.RelayError(w, http.StatusBadRequest,
			"invalid_request_error", "invalid_request_body",
			"invalid request body: "+err.Error())
		return
	}

	// Billing multipliers (n, max_tokens) are validated before any quota is
	// reserved: an out-of-bounds multiplier must be rejected here, not
	// priced.
	if err := relay.ValidateChatRequest(body); err != nil {
		common.RelayError(w, http.StatusBadRequest,
			"invalid_request_error", "invalid_request_body",
			err.Error())
		return
	}

	preQuota, err := relay.PreConsumeQuota(userID, tokenID, tokenUnlimited, modelName, group, body)
	if err != nil {
		if errors.Is(err, relay.ErrInsufficientTokenQuota) || errors.Is(err, relay.ErrInsufficientUserQuota) {
			common.RelayError(w, http.StatusForbidden,
				"insufficient_quota", "insufficient_quota", err.Error())
		} else {
			common.RelayError(w, http.StatusInternalServerError,
				"server_error", "pre_consume_failed", err.Error())
		}
		return
	}
	tried := make(map[int64]bool) // channel IDs attempted in this request
	var channelsTrail []string    // "id->id->id" retry trace for logs
	settled := false              // true once the pre-payment has been balanced by PostConsumeQuota
	defer func() {
		if settled {
			return
		}
		if err := relay.RefundPreConsumedQuota(tokenID, userID, tokenUnlimited, preQuota); err != nil {
			log.Printf("refund after failed relay failed: %v", err)
		} else {
			log.Printf("relay for model %s failed on channels [%s], refunded %d quota",
				modelName, strings.Join(channelsTrail, "->"), preQuota)
		}
	}()

	var lastErr error

	for retry := 0; retry <= common.RetryTimes; retry++ {
		if retry > 0 {
			// The previous channel failed on the upstream side; degrade to the
			// next available priority tier.
			ch, err = model.GetSatisfiedChannelExcluding(group, modelName, tried)
			if err != nil {
				break // every candidate has been tried
			}
		}
		tried[ch.ID] = true
		channelsTrail = append(channelsTrail, strconv.FormatInt(ch.ID, 10))

		res := forwardToChannel(ctx, w, ch, outBody, stream)
		switch res.outcome {
		case attemptSuccess:
			usage := res.usage
			if usage == nil || (usage.PromptTokens == 0 && usage.CompletionTokens == 0) {
				usage = relay.EstimateUsage(body, res.content)
			}
			if err := relay.PostConsumeQuota(userID, tokenID, tokenName, tokenUnlimited,
				ch.ID, modelName, group, usage, preQuota, strings.Join(channelsTrail, "->")); err != nil {
				log.Printf("post consume quota failed: %v", err)
			}
			settled = true
			return
		case attemptClientError:
			// The upstream 4xx was passed through to the client; the deferred
			// refund returns the pre-payment. No other channel is tried.
			return
		default:
			lastErr = res.cause
		}
	}

	common.RelayError(w, http.StatusBadGateway,
		"server_error", "upstream_unavailable",
		fmt.Sprintf("all %d channel attempt(s) failed for model %s (channels: %s): %v",
			len(channelsTrail), modelName, strings.Join(channelsTrail, "->"), lastErr))
}

// forwardToChannel relays the converted request body to one channel. On
// success the upstream response is written back to the client. On a 4xx the
// upstream response is passed through unchanged (and the offending key /
// channel is auto-disabled when the error proves it is broken). On 5xx /
// timeout / network errors nothing is written and the caller may retry
// another channel. The body is fully cached by the distributor, so every
// attempt replays it from the beginning.
func forwardToChannel(ctx context.Context, w http.ResponseWriter, ch *model.Channel,
	outBody []byte, stream bool) attemptResult {

	adaptor, err := relay.GetAdaptor(ch.Type)
	if err != nil {
		return attemptResult{outcome: attemptUpstreamError,
			cause: fmt.Errorf("channel %d: %v", ch.ID, err)}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		adaptor.RequestURL(ch.BaseURL), bytes.NewReader(outBody))
	if err != nil {
		return attemptResult{outcome: attemptUpstreamError,
			cause: fmt.Errorf("channel %d: build request: %v", ch.ID, err)}
	}
	upstreamKey, keyIdx := ch.SelectKey()
	adaptor.SetupRequestHeader(req, upstreamKey)

	resp, err := upstreamHTTPClient.Do(req)
	if err != nil {
		return attemptResult{outcome: attemptUpstreamError,
			cause: fmt.Errorf("channel %d unreachable: %v", ch.ID, err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if resp.StatusCode < http.StatusInternalServerError {
			// Client-class error: terminate, pass the upstream answer through.
			// A broken key must not poison later requests, though. On
			// multi-key channels only the offending key is disabled —
			// including a throttled key (429) — so the channel survives
			// while other keys remain.
			disableKey := service.ShouldDisableChannel(resp.StatusCode, string(body)) ||
				(ch.IsMultiKey() && resp.StatusCode == http.StatusTooManyRequests)
			if disableKey {
				disableBrokenKey(ch, keyIdx, resp.StatusCode)
			}
			writeUpstreamResponse(w, resp, body)
			return attemptResult{outcome: attemptClientError}
		}
		// Server-class error: switch to another channel.
		return attemptResult{outcome: attemptUpstreamError,
			cause: fmt.Errorf("channel %d returned %d", ch.ID, resp.StatusCode)}
	}

	usage, content, err := adaptor.HandleResponse(w, resp, stream)
	if err != nil {
		log.Printf("relay response handling error: %v", err)
	}
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

func writeUpstreamResponse(w http.ResponseWriter, resp *http.Response, body []byte) {
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
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

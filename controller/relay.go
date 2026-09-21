package controller

import (
	"bytes"
	"errors"
	"io"
	"log"
	"net/http"
	"time"

	"starhub/common"
	"starhub/model"
	"starhub/relay"
)

var upstreamHTTPClient = &http.Client{Timeout: 10 * time.Minute}

func Relay(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ch, _ := ctx.Value(common.CtxSelectedChannel).(*common.SelectedChannel)
	modelName, _ := ctx.Value(common.CtxRequestModel).(string)
	body, _ := ctx.Value(common.CtxRequestBody).([]byte)
	stream, _ := ctx.Value(common.CtxRequestStream).(bool)
	group, _ := ctx.Value(common.CtxUsingGroup).(string)
	userID, _ := ctx.Value(common.CtxUserID).(int64)
	tokenID, _ := ctx.Value(common.CtxTokenID).(int64)
	tokenName, _ := ctx.Value(common.CtxTokenName).(string)
	tokenUnlimited, _ := ctx.Value(common.CtxTokenUnlimited).(bool)

	if ch == nil || modelName == "" {
		common.RelayError(w, http.StatusInternalServerError,
			"server_error", "internal_error", "channel context is missing")
		return
	}

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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		adaptor.RequestURL(ch.BaseURL), bytes.NewReader(outBody))
	if err != nil {
		_ = relay.RefundPreConsumedQuota(tokenID, userID, tokenUnlimited, preQuota)
		common.RelayError(w, http.StatusInternalServerError,
			"server_error", "build_request_failed", err.Error())
		return
	}
	adaptor.SetupRequestHeader(req, ch.Key)

	resp, err := upstreamHTTPClient.Do(req)
	if err != nil {
		_ = relay.RefundPreConsumedQuota(tokenID, userID, tokenUnlimited, preQuota)
		common.RelayError(w, http.StatusBadGateway,
			"server_error", "upstream_unreachable",
			"upstream request failed: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		passThroughUpstreamResponse(w, resp)
		_ = relay.RefundPreConsumedQuota(tokenID, userID, tokenUnlimited, preQuota)
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			if err := model.UpdateChannelStatus(ch.ID, common.ChannelStatusAutoDisabled); err != nil {
				log.Printf("auto-disable channel %d failed: %v", ch.ID, err)
			} else {
				log.Printf("channel %d auto-disabled: upstream returned %d", ch.ID, resp.StatusCode)
			}
		}
		return
	}

	usage, content, err := adaptor.HandleResponse(w, resp, stream)
	if err != nil {
		log.Printf("relay response handling error: %v", err)
	}
	if usage == nil || (usage.PromptTokens == 0 && usage.CompletionTokens == 0) {
		usage = relay.EstimateUsage(body, content)
	}
	if err := relay.PostConsumeQuota(userID, tokenID, tokenName, tokenUnlimited,
		ch.ID, modelName, group, usage, preQuota); err != nil {
		log.Printf("post consume quota failed: %v", err)
	}
}

func passThroughUpstreamResponse(w http.ResponseWriter, resp *http.Response) {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
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

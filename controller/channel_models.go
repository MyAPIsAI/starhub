package controller

import (
	"errors"
	"net/http"
	"strings"

	"starhub/common"
	"starhub/model"
	"starhub/relay/channel"
	"starhub/service"
)

// FetchChannelModels previews draft connection settings without saving them.
func FetchChannelModels(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ChannelID int64   `json:"channel_id"`
		Type      int     `json:"type"`
		BaseURL   *string `json:"base_url"`
		Key       string  `json:"key"`
	}
	if !decodeJSONBody(w, r, &input) {
		return
	}
	if input.ChannelID < 0 {
		common.ApiError(w, http.StatusBadRequest, "invalid_models_channel_id")
		return
	}
	var saved *model.Channel
	if input.ChannelID > 0 {
		var err error
		saved, err = model.GetChannelByID(input.ChannelID)
		if err != nil {
			common.ApiError(w, http.StatusNotFound, "models_channel_unavailable")
			return
		}
		if input.Type == 0 {
			input.Type = saved.Type
		}
	}
	if input.Type != channel.ChannelTypeOpenAI {
		common.ApiError(w, http.StatusBadRequest, "models_channel_type_unsupported")
		return
	}
	baseURL := ""
	if saved != nil {
		baseURL = saved.BaseURL
	}
	if input.BaseURL != nil {
		baseURL = *input.BaseURL
	}
	base, err := service.NormalizeChannelModelsBaseURL(baseURL)
	if err != nil {
		common.ApiError(w, http.StatusBadRequest, err.Error())
		return
	}
	key := strings.TrimSpace(input.Key)
	if key == "" && saved != nil {
		savedBase, err := service.NormalizeChannelModelsBaseURL(saved.BaseURL)
		if err != nil || savedBase != base || saved.Type != input.Type {
			common.ApiError(w, http.StatusBadRequest, "models_changed_connection_requires_key")
			return
		}
		statuses := saved.KeyStatuses()
		for i, candidate := range saved.Keys() {
			if statuses[i] == common.StatusEnabled {
				key = candidate
				break
			}
		}
	} else if key != "" {
		// Draft multi-key input uses one explicit credential, not a permission union.
		key = strings.TrimSpace(strings.SplitN(key, "\n", 2)[0])
	}
	if key == "" {
		common.ApiError(w, http.StatusBadRequest, "models_key_required")
		return
	}
	models, err := service.FetchChannelModels(r.Context(), base, key)
	if err != nil {
		var failure *service.ChannelModelsError
		if errors.As(err, &failure) {
			common.ApiError(w, failure.Status, failure.Code)
		} else {
			common.ApiError(w, http.StatusBadGateway, "upstream_models_failed")
		}
		return
	}
	common.ApiSuccess(w, models)
}

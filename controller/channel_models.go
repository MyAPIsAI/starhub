package controller

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"

	"starhub/common"
	"starhub/model"
	"starhub/relay"
	"starhub/relay/channel"
	"starhub/service"
)

// FetchChannelModels previews draft connection settings without saving them.
func FetchChannelModels(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ChannelID       int64   `json:"channel_id"`
		TaskPluginID    int64   `json:"task_plugin_id"`
		Type            int     `json:"type"`
		BaseURL         *string `json:"base_url"`
		Key             string  `json:"key"`
		KeyIndex        *int    `json:"key_index"`
		IncludeMetadata bool    `json:"include_metadata"`
	}
	if !decodeJSONBody(w, r, &input) {
		return
	}
	if input.ChannelID < 0 {
		common.ApiError(w, http.StatusBadRequest, "invalid_models_channel_id")
		return
	}
	if input.KeyIndex != nil && *input.KeyIndex < 0 {
		common.ApiError(w, http.StatusBadRequest, "models_key_selection_invalid")
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
	if input.Type != channel.ChannelTypeOpenAI && input.Type != channel.ChannelTypeTaskPlugin && input.Type != channel.ChannelTypeArk {
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
	auth := "bearer"
	normalizeBase := service.NormalizeChannelModelsBaseURL
	fetchModels := service.FetchChannelModelsWithAuth
	if input.Type == channel.ChannelTypeArk {
		normalizeBase = service.NormalizeArkModelsBaseURL
		fetchModels = service.FetchArkChannelModels
	}
	var pluginModels []string
	if input.Type == channel.ChannelTypeTaskPlugin {
		normalizeBase = service.NormalizeTaskPluginModelsBaseURL
		if input.TaskPluginID == 0 && saved != nil && saved.Type == input.Type {
			input.TaskPluginID = saved.TaskPluginID
		}
		plugin, err := relay.LoadTaskPlugin(r.Context(), input.TaskPluginID, false)
		if err != nil {
			common.ApiError(w, http.StatusBadRequest, "models_plugin_unavailable")
			return
		}
		if strings.TrimSpace(baseURL) == "" {
			common.ApiError(w, http.StatusBadRequest, "invalid_models_base_url")
			return
		}
		auth, pluginModels = plugin.Meta.Auth, plugin.Meta.Models
		fetchModels = func(ctx context.Context, base, key, auth string) ([]string, error) {
			return service.FetchTaskPluginChannelModels(ctx, base, key, auth, plugin.Meta.ModelsPath)
		}
	}
	base, err := normalizeBase(baseURL)
	if err != nil {
		common.ApiError(w, http.StatusBadRequest, err.Error())
		return
	}
	key := strings.TrimSpace(input.Key)
	if key == "" && saved != nil {
		savedBase, err := normalizeBase(saved.BaseURL)
		if err != nil || savedBase != base || saved.Type != input.Type ||
			(input.Type == channel.ChannelTypeTaskPlugin && saved.TaskPluginID != input.TaskPluginID) {
			common.ApiError(w, http.StatusBadRequest, "models_changed_connection_requires_key")
			return
		}
		if input.KeyIndex != nil {
			var ok bool
			key, ok = saved.KeyAt(*input.KeyIndex)
			if !ok {
				common.ApiError(w, http.StatusBadRequest, "models_key_selection_invalid")
				return
			}
		} else {
			statuses := saved.KeyStatuses()
			for i, candidate := range saved.Keys() {
				if statuses[i] == common.StatusEnabled {
					key = candidate
					break
				}
			}
		}
	} else if key != "" {
		// Draft multi-key input uses one explicit credential, not a permission union.
		keys := (&model.Channel{Key: key}).Keys()
		index := 0
		if input.KeyIndex != nil {
			index = *input.KeyIndex
		}
		if index >= len(keys) {
			common.ApiError(w, http.StatusBadRequest, "models_key_selection_invalid")
			return
		}
		key = keys[index]
	}
	if key == "" {
		common.ApiError(w, http.StatusBadRequest, "models_key_required")
		return
	}
	var models []string
	var catalog []service.ChannelModelInfo
	if input.IncludeMetadata && input.Type == channel.ChannelTypeArk {
		catalog, err = service.FetchArkChannelModelCatalog(r.Context(), base, key, auth)
	} else {
		models, err = fetchModels(r.Context(), base, key, auth)
	}
	if err != nil {
		var failure *service.ChannelModelsError
		if errors.As(err, &failure) {
			common.WriteJSON(w, failure.Status, common.APIResponse{Success: false, Message: failure.Code, Data: failure.Diagnostic})
		} else {
			common.ApiError(w, http.StatusBadGateway, "upstream_models_failed")
		}
		return
	}
	if pluginModels != nil {
		models = slices.DeleteFunc(models, func(name string) bool { return !slices.Contains(pluginModels, name) })
	}
	if input.IncludeMetadata {
		if catalog == nil {
			catalog = make([]service.ChannelModelInfo, 0, len(models))
			for _, id := range models {
				catalog = append(catalog, service.ChannelModelInfo{ID: id})
			}
		}
		common.ApiSuccess(w, catalog)
		return
	}
	common.ApiSuccess(w, models)
}

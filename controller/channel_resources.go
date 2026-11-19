package controller

import (
	"errors"
	"net/http"
	"strings"

	"starhub/common"
	"starhub/model"
)

func SetChannelResourceStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		Model   string `json:"model"`
		Enabled *bool  `json:"enabled"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Enabled == nil || req.Model == "" || req.Model != strings.TrimSpace(req.Model) || len(req.Model) > 128 || strings.ContainsAny(req.Model, ",\r\n\x00") {
		common.ApiError(w, http.StatusBadRequest, "model and enabled are required")
		return
	}
	ch, err := model.SetChannelResourceEnabled(id, req.Model, *req.Enabled)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, model.ErrChannelNotFound) || errors.Is(err, model.ErrChannelResourceNotFound) {
			status = http.StatusNotFound
		}
		if errors.Is(err, model.ErrChannelResourceUnavailable) || errors.Is(err, model.ErrTaskPluginConfiguration) {
			status = http.StatusConflict
		}
		common.ApiError(w, status, err.Error())
		return
	}
	writeChannelResponses(w, []model.Channel{*ch}, true)
}

func writeChannelResponses(w http.ResponseWriter, channels []model.Channel, single bool) {
	states, err := model.ReadChannelResourceStates(channels)
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, "channel resource state is unavailable")
		return
	}
	data := make([]map[string]any, 0, len(channels))
	for i := range channels {
		row := maskedChannel(&channels[i])
		row["resource_state"] = states[channels[i].ID]
		data = append(data, row)
	}
	if single {
		common.ApiSuccess(w, data[0])
	} else {
		common.ApiSuccess(w, data)
	}
}

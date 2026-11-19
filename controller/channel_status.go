package controller

import (
	"errors"
	"net/http"

	"starhub/common"
	"starhub/model"
)

func SetChannelStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Enabled == nil {
		common.ApiError(w, http.StatusBadRequest, "enabled is required")
		return
	}
	ch, err := model.SetChannelResourcesEnabled(id, *req.Enabled)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, model.ErrChannelNotFound) {
			status = http.StatusNotFound
		} else if errors.Is(err, model.ErrTaskPluginConfiguration) {
			status = http.StatusBadRequest
		}
		common.ApiError(w, status, err.Error())
		return
	}
	writeChannelResponses(w, []model.Channel{*ch}, true)
}

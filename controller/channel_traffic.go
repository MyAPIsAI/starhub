package controller

import (
	"errors"
	"net/http"

	"starhub/common"
	"starhub/internal/traffic"
	"starhub/model"
)

func GetChannelTraffic(w http.ResponseWriter, r *http.Request) {
	if !requireGatewayRoot(w, r) {
		return
	}
	data, err := model.ReadTrafficConfiguration(r.Context())
	if err != nil {
		common.ApiError(w, http.StatusServiceUnavailable, "traffic configuration could not be read")
		return
	}
	common.ApiSuccess(w, data)
}

func UpdateChannelTraffic(w http.ResponseWriter, r *http.Request) {
	if !requireGatewayRoot(w, r) {
		return
	}
	var req struct {
		ExpectedVersion *int64       `json:"expected_version"`
		ChannelRevision *int64       `json:"channel_revision"`
		Rule            traffic.Rule `json:"rule"`
		Remove          bool         `json:"remove"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.ExpectedVersion == nil || req.ChannelRevision == nil {
		common.ApiError(w, http.StatusBadRequest, "configuration and channel versions are required")
		return
	}
	actor, _ := r.Context().Value(common.CtxUserID).(int64)
	version, err := model.SaveTrafficRule(r.Context(), req.Rule, req.Remove, actor, *req.ExpectedVersion, *req.ChannelRevision)
	if err != nil {
		switch {
		case errors.Is(err, model.ErrOptionVersionConflict):
			common.ApiError(w, http.StatusConflict, "configuration or channels changed; reload and review again")
		case errors.Is(err, model.ErrTrafficConfiguration), errors.Is(err, model.ErrTrafficMode):
			common.ApiError(w, http.StatusBadRequest, err.Error())
		default:
			common.ApiError(w, http.StatusInternalServerError, "traffic configuration could not be saved")
		}
		return
	}
	applied := model.LoadOptionsToSettings() == nil
	common.ApiSuccess(w, map[string]any{"version": version, "applied": applied})
}

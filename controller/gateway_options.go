package controller

import (
	"errors"
	"net/http"

	"starhub/common"
	"starhub/model"
)

func requireGatewayRoot(w http.ResponseWriter, r *http.Request) bool {
	role, _ := r.Context().Value(common.CtxRole).(int)
	if role != common.RoleRoot {
		common.ApiError(w, http.StatusForbidden, "root privilege required for gateway settings")
		return false
	}
	return true
}

func GetGatewayOptions(w http.ResponseWriter, r *http.Request) {
	if !requireGatewayRoot(w, r) {
		return
	}
	data, err := model.ReadGatewayOptions(r.Context())
	if err != nil {
		common.ApiError(w, http.StatusServiceUnavailable, "gateway settings could not be read; refresh and retry")
		return
	}
	common.ApiSuccess(w, data)
}

func UpdateGatewayOptions(w http.ResponseWriter, r *http.Request) {
	if !requireGatewayRoot(w, r) {
		return
	}
	var req struct {
		ExpectedVersion *int64         `json:"expected_version"`
		Options         []model.Option `json:"options"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.ExpectedVersion == nil {
		common.ApiError(w, http.StatusBadRequest, "expected_version is required")
		return
	}
	actor, _ := r.Context().Value(common.CtxUserID).(int64)
	version, err := model.SaveGatewayOptions(r.Context(), req.Options, actor, *req.ExpectedVersion)
	if err != nil {
		switch {
		case errors.Is(err, model.ErrInvalidGatewayOptions):
			common.ApiError(w, http.StatusBadRequest, "all four valid gateway limits are required")
		case errors.Is(err, model.ErrOptionVersionConflict):
			common.ApiError(w, http.StatusConflict, "configuration changed; reload and review again")
		default:
			common.ApiError(w, http.StatusInternalServerError, "gateway settings could not be saved")
		}
		return
	}
	// A committed write must not be reported as an uncommitted failure.
	applied := model.LoadOptionsToSettings() == nil
	common.ApiSuccess(w, map[string]any{"version": version, "applied": applied})
}

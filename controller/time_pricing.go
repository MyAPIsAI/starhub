package controller

import (
	"encoding/json"
	"errors"
	"net/http"

	"starhub/common"
	"starhub/model"
	"starhub/pkg/pricingtime"
)

func GetTimePricing(w http.ResponseWriter, r *http.Request) {
	state, err := model.ReadTimePricing(r.Context())
	if err != nil {
		common.ApiError(w, 500, "stored time-pricing configuration is unavailable")
		return
	}
	common.ApiSuccess(w, map[string]any{"version": state.Version, "rules": state.Rules, "loaded_version": model.LoadedOptionVersion(), "revision_guards": true, "schema": pricingtime.Schema, "billing_active": true})
}

func UpdateTimePricing(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ExpectedVersion *int64          `json:"expected_version"`
		Rules           json.RawMessage `json:"rules"`
	}
	if !decodeResourceBody(w, r, &request) {
		return
	}
	if request.ExpectedVersion == nil {
		common.ApiError(w, 400, "expected_version is required")
		return
	}
	actor, _ := r.Context().Value(common.CtxUserID).(int64)
	version, err := model.SaveTimePricingIfVersion(r.Context(), string(request.Rules), actor, *request.ExpectedVersion)
	if err != nil {
		switch {
		case errors.Is(err, model.ErrInvalidTimePricing):
			common.ApiError(w, 400, err.Error())
		case errors.Is(err, model.ErrOptionVersionConflict):
			common.ApiError(w, 409, "configuration changed; reload and review again")
		default:
			common.ApiError(w, 500, "time-pricing configuration could not be changed")
		}
		return
	}
	if err := model.LoadOptionsToSettings(); err != nil {
		common.ApiError(w, 500, "time-pricing configuration saved, but serving settings could not be reloaded")
		return
	}
	common.ApiSuccess(w, map[string]any{"version": version, "loaded_version": model.LoadedOptionVersion(), "schema": pricingtime.Schema, "billing_active": true})
}

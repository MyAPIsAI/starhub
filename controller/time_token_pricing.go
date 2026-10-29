package controller

import (
	"encoding/json"
	"errors"
	"net/http"

	"starhub/common"
	"starhub/model"
	"starhub/pkg/pricingtime"
)

func tokenTimePricingCapabilities(version int64) map[string]any {
	return map[string]any{"version": version, "loaded_version": model.LoadedOptionVersion(), "revision_guards": true, "schema": pricingtime.TokenPricesSchema, "billing_active": true, "token_only": true, "asynchronous_tasks": true, "task_pricing_snapshot": 2, "missing_usage_behavior": "admission_estimate", "multiplier_behavior": "stacked"}
}

func GetTokenTimePricing(w http.ResponseWriter, r *http.Request) {
	state, err := model.ReadTokenTimePricing(r.Context())
	if err != nil {
		common.ApiError(w, 500, "stored independent token-pricing configuration is unavailable")
		return
	}
	response := tokenTimePricingCapabilities(state.Version)
	response["rules"] = state.Rules
	common.ApiSuccess(w, response)
}

func UpdateTokenTimePricing(w http.ResponseWriter, r *http.Request) {
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
	version, err := model.SaveTokenTimePricingIfVersion(r.Context(), string(request.Rules), actor, *request.ExpectedVersion)
	if err != nil {
		switch {
		case errors.Is(err, model.ErrInvalidTokenTimePricing):
			common.ApiError(w, 400, err.Error())
		case errors.Is(err, model.ErrOptionVersionConflict):
			common.ApiError(w, 409, "configuration changed; reload and review again")
		default:
			common.ApiError(w, 500, "independent token-pricing configuration could not be changed")
		}
		return
	}
	if err := model.LoadOptionsToSettings(); err != nil {
		common.ApiError(w, 500, "independent token-pricing configuration saved, but serving settings could not be reloaded")
		return
	}
	common.ApiSuccess(w, tokenTimePricingCapabilities(version))
}

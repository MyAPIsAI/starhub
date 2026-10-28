package controller

import (
	"errors"
	"net/http"

	"starhub/common"
	"starhub/model"
)

func GetPricingOptions(w http.ResponseWriter, r *http.Request) {
	state, err := model.ReadPricingOptions(r.Context())
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, "stored pricing configuration is unavailable")
		return
	}
	common.ApiSuccess(w, map[string]any{"version": state.Version, "options": state.Options,
		"loaded_version": model.LoadedOptionVersion(), "atomic_batch": true})
}

func UpdatePricingOptions(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ExpectedVersion *int64         `json:"expected_version"`
		Options         []model.Option `json:"options"`
	}
	if !decodeResourceBody(w, r, &req) {
		return
	}
	if req.ExpectedVersion == nil {
		common.ApiError(w, http.StatusBadRequest, "expected_version is required")
		return
	}
	actor, _ := r.Context().Value(common.CtxUserID).(int64)
	version, err := model.SavePricingOptionsIfVersion(r.Context(), req.Options, actor, *req.ExpectedVersion)
	if err != nil {
		switch {
		case errors.Is(err, model.ErrInvalidPricingOptions):
			common.ApiError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, model.ErrOptionVersionConflict):
			common.ApiError(w, http.StatusConflict, "configuration changed; reload and review again")
		default:
			common.ApiError(w, http.StatusInternalServerError, "pricing configuration could not be changed")
		}
		return
	}
	if err := model.LoadOptionsToSettings(); err != nil {
		common.ApiError(w, http.StatusInternalServerError, "pricing configuration saved, but serving settings could not be reloaded")
		return
	}
	common.ApiSuccess(w, map[string]any{"version": version, "loaded_version": model.LoadedOptionVersion()})
}

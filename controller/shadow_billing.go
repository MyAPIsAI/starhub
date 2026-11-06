package controller

import (
	"net/http"
	"time"

	"starhub/common"
	"starhub/service"
)

type shadowBillingRequest struct {
	Hours int `json:"hours"`
	Limit int `json:"limit"`
}

// ShadowBillingHandler handles POST /api/shadow-billing (AdminAuth).
// Runs an offline billing verification by recomputing quota from Log
// records and comparing with the stored values.
func ShadowBillingHandler(w http.ResponseWriter, r *http.Request) {
	var req shadowBillingRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Hours <= 0 {
		req.Hours = 24
	}
	if req.Hours > 720 {
		req.Hours = 720 // max 30 days
	}
	if req.Limit <= 0 {
		req.Limit = 1000
	}

	since := time.Now().Add(-time.Duration(req.Hours) * time.Hour).Unix()
	report, err := service.RunShadowBillingVerification(since, req.Limit)
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.ApiSuccess(w, report)
}

package controller

import (
	"net/http"
	"strconv"
	"time"

	"starhub/common"
	"starhub/model"
	"starhub/service/authz"
)

func GetFinanceSummary(w http.ResponseWriter, r *http.Request) {
	if !authz.Allow(requestRole(r), authz.ResourceFinance, authz.ActionRead) {
		common.ApiError(w, http.StatusForbidden, "finance privilege required")
		return
	}
	days := 30
	if raw := r.URL.Query().Get("days"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 90 {
			common.ApiError(w, http.StatusBadRequest, "days must be an integer between 1 and 90")
			return
		}
		days = parsed
	}
	until := time.Now().Unix() + 1
	result, err := model.ReadFinanceSummary(r.Context(), until-int64(days)*86400, until)
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, "failed to read the finance summary")
		return
	}
	common.ApiSuccess(w, result)
}

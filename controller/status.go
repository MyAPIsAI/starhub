package controller

import (
	"net/http"
	"strconv"

	"starhub/common"
	"starhub/model"
)

// StatusHistoryHandler handles GET /api/status/history?months=3 (public).
// Returns monthly availability aggregated from the Log table.
func StatusHistoryHandler(w http.ResponseWriter, r *http.Request) {
	months, _ := strconv.Atoi(r.URL.Query().Get("months"))
	if months <= 0 {
		months = 3
	}
	// Try persisted records first.
	records, err := model.ListAvailabilityHistory(months)
	if err == nil && len(records) > 0 {
		common.ApiSuccess(w, records)
		return
	}
	// Fall back to real-time aggregation from logs.
	records, err = model.AggregateMonthlyAvailability(months)
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.ApiSuccess(w, records)
}

package controller

import (
	"net/http"
	"strconv"
	"time"

	"starhub/common"
	"starhub/model"
)

// GetServiceQuality returns per-model quality metrics aggregated from
// the Log table. Users see their own; admins may select an account or see all.
func GetServiceQuality(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	hours, _ := strconv.ParseInt(q.Get("hours"), 10, 64)
	if hours < 1 {
		hours = 24
	}
	userID := int64(0)
	if common.IsAdminRole(requestRole(r)) {
		if value := q.Get("user_id"); value != "" {
			var err error
			userID, err = strconv.ParseInt(value, 10, 64)
			if err != nil || userID <= 0 {
				common.ApiError(w, http.StatusBadRequest, "user_id must be a positive integer")
				return
			}
		}
	} else {
		userID = requestUserID(r)
	}
	var summary *model.ServiceQualitySummary
	var err error
	if q.Has("month") {
		month, parseErr := time.Parse("2006-01", q.Get("month"))
		if parseErr != nil || month.Year() < 1970 || month.After(time.Now().UTC()) {
			common.ApiError(w, http.StatusBadRequest, "month must be YYYY-MM between 1970-01 and the current UTC month")
			return
		}
		summary, err = model.SummarizeServiceQualityForMonth(userID, month, q.Get("model"))
	} else {
		summary, err = model.SummarizeServiceQuality(userID, int(hours), q.Get("model"))
	}
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.ApiSuccess(w, summary)
}

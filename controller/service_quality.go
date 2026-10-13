package controller

import (
	"net/http"
	"strconv"

	"starhub/common"
	"starhub/model"
)

// GetServiceQuality returns per-model quality metrics aggregated from
// the Log table. Users see their own; admins see all.
func GetServiceQuality(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	hours, _ := strconv.ParseInt(q.Get("hours"), 10, 64)
	if hours < 1 {
		hours = 24
	}
	userID := int64(0)
	if requestRole(r) < common.RoleAdmin {
		userID = requestUserID(r)
	}
	summary, err := model.SummarizeServiceQuality(userID, int(hours))
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.ApiSuccess(w, summary)
}

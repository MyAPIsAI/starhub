package controller

import (
	"net/http"
	"time"

	"starhub/common"
	"starhub/model"
	"starhub/service/authz"
)

func GetOperatingDashboard(w http.ResponseWriter, r *http.Request) {
	if !authz.Allow(requestRole(r), authz.ResourceMetrics, authz.ActionRead) {
		common.ApiError(w, 403, "operating dashboard privilege required")
		return
	}
	switch r.URL.Query().Get("range") {
	case "", "today", "7d", "30d", "custom":
	default:
		common.ApiError(w, 400, "invalid dashboard range")
		return
	}
	profile, err := model.CurrentDeploymentProfile(r.Context())
	if err != nil {
		adminFailure(w, err)
		return
	}
	loc, err := time.LoadLocation(profile.Profile.Timezone)
	if err != nil {
		adminFailure(w, err)
		return
	}
	now := time.Now()
	start, until, err := reportDates(r, now, loc)
	if err != nil || start.After(now) || until.Add(-time.Second).In(loc).Format("2006-01-02") > now.In(loc).Format("2006-01-02") {
		common.ApiError(w, 400, "invalid dashboard dates; select up to one year ending today or earlier")
		return
	}
	result, err := model.ReadOperatingDashboard(r.Context(), start, until, now, profile.Profile.RegionID)
	if err != nil {
		adminFailure(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	common.ApiSuccess(w, result)
}

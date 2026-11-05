package controller

import (
	"net/http"
	"starhub/common"
	"starhub/model"
	"strconv"
	"time"
)

func AdminReconciliation(w http.ResponseWriter, r *http.Request) {
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
	end := r.URL.Query().Get("end")
	if end == "" {
		end = time.Now().In(loc).Format("2006-01-02")
	}
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days == 0 {
		days = 14
	}
	user, _ := strconv.ParseInt(r.URL.Query().Get("user_id"), 10, 64)
	at, err := time.ParseInLocation("2006-01-02", end, loc)
	if err != nil || days < 1 || days > 31 || user < 0 || end > time.Now().In(loc).Format("2006-01-02") {
		common.ApiError(w, 400, "invalid reconciliation window")
		return
	}
	if r.Method == http.MethodPost {
		for i := days - 1; i >= 0; i-- {
			if _, err := model.RunDailyReconciliation(r.Context(), at.AddDate(0, 0, -i).Format("2006-01-02"), user, requestUserID(r), time.Now()); err != nil {
				adminFailure(w, err)
				return
			}
		}
	}
	reports, err := model.ReadReconciliation(r.Context(), end, days, user)
	if err != nil {
		adminFailure(w, err)
		return
	}
	common.ApiSuccess(w, map[string]any{"items": reports, "timezone": loc.String(), "end": end, "days": days, "external_provider_verified": false})
}

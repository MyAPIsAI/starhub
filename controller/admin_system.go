package controller

import (
	"encoding/json"
	"net/http"
	"starhub/common"
	"starhub/model"
	"starhub/setting"
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
	var pending int64
	if err := model.DB.WithContext(r.Context()).Model(&model.ReconciliationAlert{}).Where("delivered_at = 0").Count(&pending).Error; err != nil {
		adminFailure(w, err)
		return
	}
	common.ApiSuccess(w, map[string]any{"items": reports, "timezone": loc.String(), "end": end, "days": days, "external_provider_verified": false, "alert_webhook_configured": setting.AlertWebhookURLValue() != "", "pending_alerts": pending})
}

func AdminPaymentStatements(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, 3*1024*1024)
		var input model.PaymentStatementImport
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			common.ApiError(w, 400, "invalid statement import")
			return
		}
		statement, err := model.ImportPaymentStatement(r.Context(), input, requestUserID(r), time.Now())
		if err != nil {
			adminFailure(w, err)
			return
		}
		common.ApiSuccess(w, statement)
		return
	}
	var items []model.PaymentStatement
	q := model.DB.WithContext(r.Context()).Order("id DESC").Limit(100)
	if day := r.URL.Query().Get("day"); day != "" {
		q = q.Where("day = ?", day)
	}
	if err := q.Find(&items).Error; err != nil {
		adminFailure(w, err)
		return
	}
	common.ApiSuccess(w, map[string]any{"items": items})
}

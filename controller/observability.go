package controller

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"starhub/common"
	"starhub/model"
	"starhub/observability"
)

func StaffObservability(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	ctx := r.Context()
	c, err := observability.LoadConfig()
	if err != nil {
		common.ApiError(w, 503, "monitoring configuration unavailable")
		return
	}
	all, err := model.ReadRequestMetrics(ctx, now, "all")
	if err != nil {
		adminFailure(w, err)
		return
	}
	dimensions, err := model.ReadRequestMetrics(ctx, now, "dimensions")
	if err != nil {
		adminFailure(w, err)
		return
	}
	pulses := []model.ObservabilityPulse{}
	if err = model.DB.WithContext(ctx).Order("at DESC").Limit(100).Find(&pulses).Error; err != nil {
		adminFailure(w, err)
		return
	}
	counts := map[string]int64{}
	for _, severity := range []string{"critical", "serious", "general"} {
		var n int64
		if err = model.DB.WithContext(ctx).Model(&model.OperationalAlert{}).Where("active = ? AND severity = ?", true, severity).Count(&n).Error; err != nil {
			adminFailure(w, err)
			return
		}
		counts[severity] = n
	}
	var pending int64
	if err = model.DB.WithContext(ctx).Model(&model.AlertDelivery{}).Where("delivered_at = 0").Count(&pending).Error; err != nil {
		adminFailure(w, err)
		return
	}
	observations := map[string]map[string]int64{}
	for _, kind := range []string{"precharge", "settlement", "ledger", "payment_callback"} {
		var total, failed int64
		q := model.DB.WithContext(ctx).Model(&model.Observation{}).Where("kind = ? AND at > ? AND at <= ?", kind, now.Add(-5*time.Minute).Unix(), now.Unix())
		if err = q.Count(&total).Error; err != nil {
			adminFailure(w, err)
			return
		}
		if err = q.Where("success = ?", false).Count(&failed).Error; err != nil {
			adminFailure(w, err)
			return
		}
		observations[kind] = map[string]int64{"total": total, "failed": failed}
	}
	var resources []model.GatewayResourceReport
	if err = model.DB.WithContext(ctx).Order("at DESC").Limit(100).Find(&resources).Error; err != nil {
		adminFailure(w, err)
		return
	}
	type gatewayCounts struct {
		ID       string `json:"gateway_id"`
		At       int64  `json:"at"`
		Open     int    `json:"open"`
		Degraded int    `json:"degraded"`
	}
	states := []gatewayCounts{}
	for _, report := range resources {
		var rows []model.ResourceLocalState
		if json.Unmarshal([]byte(report.Resources), &rows) != nil {
			continue
		}
		state := gatewayCounts{ID: report.GatewayID, At: report.At}
		for _, row := range rows {
			if row.State == "open" {
				state.Open++
			}
			if row.State == "degraded" {
				state.Degraded++
			}
		}
		states = append(states, state)
	}
	syncView, err := model.ReadResourceSyncStatus(ctx, now)
	if err != nil {
		adminFailure(w, err)
		return
	}
	common.ApiSuccess(w, map[string]any{"at": now.Unix(), "window_seconds": 300, "all": all, "dimensions": dimensions, "instances": pulses, "routing_states": states, "observations": observations, "active": counts, "pending_deliveries": pending, "sync": syncView, "notifications": map[string]any{"phone_configured": c.PhoneURL != "", "im_configured": c.IMURL != "", "general_configured": model.NotificationEndpoint(c, "general") != "", "observer_configured": c.ObserverToken != "", "timezone": c.Timezone, "work_start": c.WorkStart, "work_end": c.WorkEnd, "model_success_threshold": c.ModelSuccess, "payment_failure_threshold": c.PaymentFailure, "minimum_samples": c.MinimumSamples}})
}

func StaffOperationalAlerts(w http.ResponseWriter, r *http.Request) {
	page, size := 1, 20
	if text := r.URL.Query().Get("page"); text != "" {
		n, err := strconv.Atoi(text)
		if err != nil || n < 1 || n > 100000 {
			common.ApiError(w, 400, "invalid page")
			return
		}
		page = n
	}
	q := model.DB.WithContext(r.Context()).Model(&model.OperationalAlert{}).Where("generation > 0")
	if severity := r.URL.Query().Get("severity"); severity != "" {
		if severity != "critical" && severity != "serious" && severity != "general" {
			common.ApiError(w, 400, "invalid severity")
			return
		}
		q = q.Where("severity = ?", severity)
	}
	if state := r.URL.Query().Get("state"); state != "" {
		if state != "active" && state != "resolved" {
			common.ApiError(w, 400, "invalid state")
			return
		}
		q = q.Where("active = ?", state == "active")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		adminFailure(w, err)
		return
	}
	page = min(page, max(1, int((total+int64(size)-1)/int64(size))))
	rows := []model.OperationalAlert{}
	if err := q.Order("active DESC, last_seen DESC, id").Offset((page - 1) * size).Limit(size).Find(&rows).Error; err != nil {
		adminFailure(w, err)
		return
	}
	ids := []string{}
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	deliveries := []model.AlertDelivery{}
	if len(ids) > 0 {
		if err := model.DB.WithContext(r.Context()).Where("alert_id IN ?", ids).Order("created_at DESC, id").Limit(200).Find(&deliveries).Error; err != nil {
			adminFailure(w, err)
			return
		}
	}
	common.ApiSuccess(w, map[string]any{"items": rows, "deliveries": deliveries, "total": total, "page": page, "page_size": size})
}

func StaffTrace(w http.ResponseWriter, r *http.Request) {
	trace, err := model.ReadTrace(r.Context(), r.PathValue("trace"))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		common.ApiError(w, 404, "Trace not found in the retained telemetry window")
		return
	}
	if err != nil {
		adminFailure(w, err)
		return
	}
	common.ApiSuccess(w, trace)
}

func observerAuthorized(r *http.Request) bool {
	c, err := observability.LoadConfig()
	if err != nil || c.ObserverToken == "" {
		return false
	}
	provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	return subtle.ConstantTimeCompare([]byte(c.ObserverToken), []byte(provided)) == 1
}

func ObserverHealth(w http.ResponseWriter, r *http.Request) {
	if !observerAuthorized(r) {
		http.Error(w, "Unauthorized", 401)
		return
	}
	var one int
	err := model.DB.WithContext(r.Context()).Raw("SELECT 1").Scan(&one).Error
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(503)
	}
	_ = json.NewEncoder(w).Encode(map[string]bool{"gateway": true, "database": err == nil && one == 1})
}

func ObservePaymentCallback(w http.ResponseWriter, r *http.Request) {
	if !observerAuthorized(r) {
		common.ApiError(w, 401, "Unauthorized")
		return
	}
	var input struct {
		EventID string `json:"event_id"`
		Source  string `json:"source"`
		Success *bool  `json:"success"`
		At      int64  `json:"at"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || len(input.EventID) < 16 || len(input.EventID) > 64 || observability.SafeCode(input.EventID) != input.EventID || observability.SafeCode(input.Source) != input.Source || input.Source == "" || input.Success == nil || input.At < time.Now().Add(-time.Minute).Unix() || input.At > time.Now().Add(time.Minute).Unix() {
		common.ApiError(w, 400, "invalid callback observation")
		return
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		common.ApiError(w, 400, "invalid callback observation")
		return
	}
	raw, _ := json.Marshal(input)
	hash := sha256.Sum256(raw)
	receipt := model.ObservationReceipt{ID: input.EventID, Digest: hex.EncodeToString(hash[:])}
	err := model.DB.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&receipt)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			var old model.ObservationReceipt
			if err := tx.First(&old, "id = ?", receipt.ID).Error; err != nil {
				return err
			}
			if old.Digest != receipt.Digest {
				return model.ErrResourceConflict
			}
			return nil
		}
		return tx.Create(&model.Observation{Kind: "payment_callback", Source: input.Source, Success: *input.Success, At: input.At}).Error
	})
	if errors.Is(err, model.ErrResourceConflict) {
		common.ApiError(w, 409, "observation identity conflict")
		return
	}
	if err != nil {
		adminFailure(w, err)
		return
	}
	common.ApiSuccess(w, map[string]bool{"recorded": true})
}

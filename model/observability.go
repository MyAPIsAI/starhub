package model

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"starhub/observability"
)

type TraceRequest observability.Request
type TraceStep observability.Step

func TraceVendor(name string) string {
	if meta, found, err := GetModelMeta(name); err == nil && found && meta.Vendor != "" {
		return meta.Vendor
	}
	vendor, _ := InferVendor(name)
	return vendor
}

type ObservationReceipt struct {
	ID     string `gorm:"primaryKey;size:64"`
	Digest string
}

type Observation struct {
	ID      int64  `gorm:"primaryKey" json:"id"`
	Kind    string `gorm:"index:idx_observation_window,priority:1" json:"kind"`
	Source  string `json:"source"`
	Success bool   `json:"success"`
	At      int64  `gorm:"index:idx_observation_window,priority:2" json:"at"`
}

type ObservabilityPulse struct {
	ID     string   `gorm:"primaryKey" json:"gateway_id"`
	At     int64    `json:"at"`
	CPU    *float64 `json:"cpu_percent"`
	Memory *float64 `json:"memory_percent"`
	Disk   *float64 `json:"disk_percent"`
}

func SaveTrace(ctx context.Context, request observability.Request, steps []observability.Step) error {
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row := TraceRequest(request)
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
		for _, step := range steps {
			row := TraceStep(step)
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// RecordTraceStep runs outside accounting transactions; telemetry never changes money outcomes.
func RecordTraceStep(trace, stage, status, code string, reference int64, started time.Time) {
	if trace == "" {
		return
	}
	step := TraceStep{ID: observability.NewID(), TraceID: trace, Stage: stage, Status: status, Code: observability.SafeCode(code), ReferenceID: reference, StartedAt: started.UnixMilli(), EndedAt: time.Now().UnixMilli()}
	observability.Log(trace, stage, status)
	if DB != nil {
		if err := DB.Create(&step).Error; err != nil {
			observability.Log(trace, "telemetry_write", "failed")
		}
	}
}

type TraceView struct {
	Request          TraceRequest `json:"request"`
	Steps            []TraceStep  `json:"steps"`
	LogID            int64        `json:"log_id"`
	LedgerID         int64        `json:"ledger_id"`
	SettlementID     int64        `json:"settlement_id"`
	AccountingStatus string       `json:"accounting_status"`
	ChargedNano      string       `json:"charged_nano"`
	Complete         bool         `json:"complete"`
}

func ReadTrace(ctx context.Context, id string) (*TraceView, error) {
	if len(id) < 16 || len(id) > 64 || observability.SafeCode(id) != id {
		return nil, gorm.ErrRecordNotFound
	}
	result := &TraceView{Steps: []TraceStep{}, AccountingStatus: "not_applicable", ChargedNano: "0"}
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&result.Request, "id = ?", id).Error; err != nil {
			return err
		}
		if err := tx.Where("trace_id = ?", id).Order("started_at, ended_at, id").Limit(512).Find(&result.Steps).Error; err != nil {
			return err
		}
		var entry Log
		err := tx.Where("trace_id = ?", id).First(&entry).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil {
			result.LogID = entry.ID
			result.AccountingStatus = "no_charge"
			var charge APIChargeEvent
			if err := tx.Where("log_id = ?", entry.ID).Limit(1).Find(&charge).Error; err != nil {
				return err
			}
			if charge.LogID != 0 {
				result.LedgerID = charge.LedgerID
				result.ChargedNano = fmt.Sprint(charge.Amount)
				result.AccountingStatus = "pending_ledger"
				if charge.LedgerID > 0 {
					result.AccountingStatus = "posted"
				}
			}
		}
		var intent QuotaSettlement
		if err := tx.Where("json_extract(payload, '$.TraceID') = ? OR json_extract(payload, '$.trace_id') = ?", id, id).Order("id DESC").Limit(1).Find(&intent).Error; err != nil {
			return err
		}
		if intent.ID > 0 {
			result.SettlementID = intent.ID
			if intent.CompletedAt == 0 {
				result.AccountingStatus = "pending_settlement"
			}
		}
		if result.LogID == 0 && intent.ID == 0 {
			for _, step := range result.Steps {
				if (step.Stage == "settlement" || step.Stage == "refund") && step.Status == "failed" {
					result.AccountingStatus = "accounting_failed"
				}
			}
		}
		var task Task
		if err := tx.Where("trace_id = ?", id).Order("id DESC").Limit(1).Find(&task).Error; err != nil {
			return err
		}
		if task.ID > 0 && task.Status != "succeeded" && task.Status != "failed" && task.Status != "cancelled" {
			result.AccountingStatus = "pending_task"
		}
		result.Complete = result.Request.EndedAt > 0 && result.AccountingStatus != "pending_settlement" && result.AccountingStatus != "pending_ledger" && result.AccountingStatus != "pending_task" && result.AccountingStatus != "accounting_failed"
		return nil
	})
	return result, err
}

type RequestMetric struct {
	Protocol     string   `json:"protocol"`
	Model        string   `json:"model"`
	Vendor       string   `json:"vendor"`
	ServiceGroup string   `json:"service_group"`
	Requests     int64    `json:"requests"`
	Eligible     int64    `json:"eligible"`
	Failures     int64    `json:"failures"`
	SuccessRate  *float64 `json:"success_rate"`
	TTFTMS       float64  `json:"ttft_ms"`
	DurationMS   float64  `json:"duration_ms"`
	GatewayMS    float64  `json:"gateway_ms"`
}

func ReadRequestMetrics(ctx context.Context, now time.Time, dimension string) ([]RequestMetric, error) {
	group := ""
	switch dimension {
	case "all":
	case "model":
		group = "model"
	case "vendor":
		group = "vendor"
	case "dimensions":
		group = "protocol, model, vendor, service_group"
	default:
		return nil, errors.New("invalid dimension")
	}
	selection := "COUNT(*) AS requests, SUM(CASE WHEN cancelled = 0 AND (status >= 500 OR failed = 1 OR (status >= 200 AND status < 400) OR status = 101) THEN 1 ELSE 0 END) AS eligible, SUM(CASE WHEN cancelled = 0 AND (status >= 500 OR failed = 1) THEN 1 ELSE 0 END) AS failures, AVG(ttftms) AS ttftms, AVG(duration_ms) AS duration_ms, AVG(gateway_ms) AS gateway_ms"
	if group != "" {
		selection = group + ", " + selection
	}
	q := DB.WithContext(ctx).Model(&TraceRequest{}).Where("ended_at > ? AND ended_at <= ?", now.Add(-5*time.Minute).UnixMilli(), now.UnixMilli()).Select(selection)
	if group != "" {
		q = q.Group(group)
	}
	rows := []RequestMetric{}
	if err := q.Order("requests DESC").Limit(500).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for i := range rows {
		if rows[i].Eligible > 0 {
			rate := float64(rows[i].Eligible-rows[i].Failures) / float64(rows[i].Eligible)
			rows[i].SuccessRate = &rate
		}
	}
	return rows, nil
}

func RecordObservation(kind, source string, success bool, now time.Time) error {
	if kind != "payment_callback" && kind != "precharge" && kind != "settlement" && kind != "ledger" {
		return errors.New("invalid observation kind")
	}
	return DB.Create(&Observation{Kind: kind, Source: observability.SafeCode(source), Success: success, At: now.Unix()}).Error
}

func recordPostedLedgerTraces(ctx context.Context) {
	type posted struct {
		TraceID   string
		LogID     int64
		LedgerID  int64
		CreatedAt int64
	}
	var rows []posted
	err := DB.WithContext(ctx).Table("api_charge_events AS e").Select("l.trace_id, e.log_id, e.ledger_id, e.created_at").Joins("JOIN logs AS l ON l.id = e.log_id").Where("e.ledger_id > 0 AND l.trace_id <> '' AND NOT EXISTS (SELECT 1 FROM trace_steps s WHERE s.id = ('ledger:' || e.log_id))").Limit(1000).Scan(&rows).Error
	if err != nil {
		return
	}
	for _, r := range rows {
		row := TraceStep{ID: fmt.Sprintf("ledger:%d", r.LogID), TraceID: r.TraceID, Stage: "ledger", Status: "success", ReferenceID: r.LedgerID, StartedAt: time.Now().UnixMilli(), EndedAt: time.Now().UnixMilli()}
		if err := DB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			observability.Log(r.TraceID, "ledger_trace", "failed")
		}
	}
}

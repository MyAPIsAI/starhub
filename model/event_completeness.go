package model

import (
	"context"
	"errors"
	"math"
	"time"

	"gorm.io/gorm"
)

// EventAuditRequest is an internal receipt, independent of the final request log.
// It contains no customer identity or content and is never exported as telemetry.
type EventAuditRequest struct {
	RequestID         string `gorm:"primaryKey;size:64"`
	StartedAt         int64  `gorm:"index"`
	CompletedAt       int64  `gorm:"index"`
	ExpectedDecisions int64
	ExpectedAttempts  int64
}

type EventAuditDecision struct {
	ID          string `gorm:"primaryKey;size:64"`
	RequestID   string `gorm:"index;size:64"`
	CompletedAt int64  `gorm:"index"`
	DurationUS  int64
}

type EventCompletenessData struct {
	Schema                    string   `json:"schema"`
	Start                     int64    `json:"window_start"`
	End                       int64    `json:"window_end"`
	Scope                     string   `json:"scope"`
	RequestEventsTotal        int64    `json:"request_events_total"`
	RequestCompleteness       *float64 `json:"request_completeness"`
	DecisionEventsTotal       int64    `json:"decision_events_total"`
	DecisionCompleteness      *float64 `json:"decision_completeness"`
	AttemptEventsTotal        int64    `json:"attempt_events_total"`
	AttemptCompleteness       *float64 `json:"attempt_completeness"`
	UnknownHistoricalRequests int64    `json:"unknown_historical_requests"`
	UnresolvedRequests        int64    `json:"unresolved_requests"`
}

func BeginEventAudit(id string, at time.Time) error {
	return DB.Create(&EventAuditRequest{RequestID: id, StartedAt: at.Unix()}).Error
}

func CompleteEventAudit(id string, decisions, attempts int64, at time.Time) error {
	return DB.Model(&EventAuditRequest{}).Where("request_id = ?", id).Updates(map[string]any{"completed_at": at.Unix(), "expected_decisions": decisions, "expected_attempts": attempts}).Error
}

// ReadEventCompleteness measures finalized requests by their completion clock.
// Historical records without receipts are unknown, never fabricated successes.
func ReadEventCompleteness(ctx context.Context, start, end int64) (EventCompletenessData, error) {
	d := EventCompletenessData{Schema: "starhub-events-v1", Start: start, End: end, Scope: "finalized_http_relay_requests"}
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var audits []EventAuditRequest
		if err := tx.Where("completed_at >= ? AND completed_at < ?", start, end).Limit(100001).Find(&audits).Error; err != nil {
			return err
		}
		if len(audits) > 100000 {
			return errors.New("event audit window exceeds query budget")
		}
		var unknown int64
		if err := tx.Model(&Log{}).Where("created_at >= ? AND created_at < ? AND NOT EXISTS (SELECT 1 FROM event_audit_requests a WHERE a.request_id = logs.request_id)", start, end).Count(&unknown).Error; err != nil {
			return err
		}
		d.UnknownHistoricalRequests = unknown
		if err := tx.Model(&EventAuditRequest{}).Where("completed_at = 0 AND started_at >= ? AND started_at < ?", start, end-600).Count(&d.UnresolvedRequests).Error; err != nil {
			return err
		}
		counts := func(table, condition string) (map[string]int64, error) {
			var rows []struct {
				RequestID string
				N         int64
			}
			q := tx.Table(table+" AS e").Select("e.request_id, COUNT(*) AS n").Joins("JOIN event_audit_requests AS a ON a.request_id = e.request_id").Where("a.completed_at >= ? AND a.completed_at < ?", start, end)
			if condition != "" {
				q = q.Where(condition)
			}
			if err := q.Group("e.request_id").Scan(&rows).Error; err != nil {
				return nil, err
			}
			result := map[string]int64{}
			for _, row := range rows {
				result[row.RequestID] = row.N
			}
			return result, nil
		}
		logs, err := counts("logs", "e.request_observed = TRUE")
		if err != nil {
			return err
		}
		decisions, err := counts("event_audit_decisions", "e.completed_at > 0")
		if err != nil {
			return err
		}
		attempts, err := counts("production_attempt_records", "e.sent = TRUE AND (e.batch_id = '' OR EXISTS (SELECT 1 FROM production_telemetry_minutes AS p WHERE p.batch_id = e.batch_id AND p.minute = e.minute))")
		if err != nil {
			return err
		}
		var requestOK, decisionOK, attemptOK int64
		for _, a := range audits {
			d.RequestEventsTotal++
			d.DecisionEventsTotal += a.ExpectedDecisions
			d.AttemptEventsTotal += a.ExpectedAttempts
			if logs[a.RequestID] == 1 {
				requestOK++
			}
			decisionOK += min(decisions[a.RequestID], a.ExpectedDecisions)
			attemptOK += min(attempts[a.RequestID], a.ExpectedAttempts)
		}
		if unknown == 0 && d.UnresolvedRequests == 0 {
			d.RequestCompleteness = eventPercent(requestOK, d.RequestEventsTotal)
			d.DecisionCompleteness = eventPercent(decisionOK, d.DecisionEventsTotal)
			d.AttemptCompleteness = eventPercent(attemptOK, d.AttemptEventsTotal)
		}
		return nil
	})
	return d, err
}

func eventPercent(n, total int64) *float64 {
	if total == 0 {
		return nil
	}
	v := math.Min(100, 100*float64(n)/float64(total))
	return &v
}

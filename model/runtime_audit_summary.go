package model

import (
	"context"
	"sort"

	"gorm.io/gorm"
)

type RuntimeAuditSummary struct {
	Start           int64    `json:"window_start"`
	End             int64    `json:"window_end"`
	ClientCount     int64    `json:"client_count"`
	AdmittedCount   int64    `json:"admitted_count"`
	SmartCount      int64    `json:"smart_count"`
	DecisionCount   int64    `json:"decision_count"`
	ClientSuccess   *float64 `json:"client_success_rate"`
	AdmittedSuccess *float64 `json:"admitted_success_rate"`
	SmartSuccess    *float64 `json:"smart_success_rate"`
	FallbackRate    *float64 `json:"fallback_rate"`
	RoutingP95      *float64 `json:"routing_latency_p95"`
	RoutingP99      *float64 `json:"routing_latency_p99"`
}

// Request outcomes and decision timings use independent populations. Provider
// TTFT is never substituted for time spent choosing a channel.
func ReadRuntimeAuditSummary(ctx context.Context, start, end int64) (*RuntimeAuditSummary, error) {
	s := &RuntimeAuditSummary{Start: start, End: end}
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var receipts []EventAuditRequest
		if err := tx.Where("completed_at >= ? AND completed_at < ?", start, end).Limit(100001).Find(&receipts).Error; err != nil {
			return err
		}
		if len(receipts) > 100000 {
			return nil
		}
		var logs []Log
		if err := tx.Select("logs.request_id, logs.status, logs.admission_rejected, logs.routing_fallback").Joins("JOIN event_audit_requests a ON a.request_id = logs.request_id").Where("a.completed_at >= ? AND a.completed_at < ? AND logs.request_observed = TRUE", start, end).Find(&logs).Error; err != nil {
			return err
		}
		byRequest := map[string][]Log{}
		for _, l := range logs {
			byRequest[l.RequestID] = append(byRequest[l.RequestID], l)
		}
		var decisions []EventAuditDecision
		if err := tx.Joins("JOIN event_audit_requests a ON a.request_id = event_audit_decisions.request_id").Where("a.completed_at >= ? AND a.completed_at < ? AND event_audit_decisions.completed_at > 0", start, end).Limit(100001).Find(&decisions).Error; err != nil {
			return err
		}
		smart := map[string]bool{}
		var timings []int64
		for _, d := range decisions {
			smart[d.RequestID] = smart[d.RequestID] || d.Smart
			if d.DurationUS >= 0 {
				timings = append(timings, d.DurationUS)
			}
		}
		var clientSuccess, admittedSuccess, smartSuccess, fallback, expectedDecisions int64
		complete := true
		for _, a := range receipts {
			s.ClientCount++
			expectedDecisions += a.ExpectedDecisions
			rows := byRequest[a.RequestID]
			if len(rows) != 1 {
				complete = false
				continue
			}
			l := rows[0]
			if l.Status == 1 {
				clientSuccess++
			}
			if !l.AdmissionRejected {
				s.AdmittedCount++
				if l.Status == 1 {
					admittedSuccess++
				}
			}
			if smart[a.RequestID] {
				s.SmartCount++
				if l.Status == 1 {
					smartSuccess++
				}
			}
			if l.RoutingFallback {
				fallback++
			}
		}
		var historical int64
		if err := tx.Model(&Log{}).Where("created_at >= ? AND created_at < ? AND NOT EXISTS (SELECT 1 FROM event_audit_requests a WHERE a.request_id = logs.request_id)", start, end).Count(&historical).Error; err != nil {
			return err
		}
		if complete && historical == 0 {
			s.ClientSuccess = eventPercent(clientSuccess, s.ClientCount)
			s.AdmittedSuccess = eventPercent(admittedSuccess, s.AdmittedCount)
			s.FallbackRate = eventPercent(fallback, s.ClientCount)
			if int64(len(decisions)) == expectedDecisions {
				s.SmartSuccess = eventPercent(smartSuccess, s.SmartCount)
			}
		}
		if int64(len(timings)) == expectedDecisions && len(timings) > 0 && len(timings) <= 100000 {
			sort.Slice(timings, func(i, j int) bool { return timings[i] < timings[j] })
			p95 := float64(timings[(len(timings)*95+99)/100-1]) / 1000
			p99 := float64(timings[(len(timings)*99+99)/100-1]) / 1000
			s.RoutingP95, s.RoutingP99 = &p95, &p99
			s.DecisionCount = int64(len(timings))
		}
		return nil
	})
	return s, err
}

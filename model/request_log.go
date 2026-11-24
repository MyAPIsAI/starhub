package model

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

type LogFilters struct {
	UserID int64
	Model  string
	Status int
	From   int64
	Until  int64
	Query  string
}

type LogPageSummary struct {
	ObservedRequests   int64 `json:"observed_requests"`
	RejectedRequests   int64 `json:"rejected_requests"`
	UpstreamAttempts   int64 `json:"upstream_attempts"`
	HistoricalRequests int64 `json:"historical_requests"`
	SuccessCount       int64 `json:"success_count"`
	FailedCount        int64 `json:"failed_count"`
	CancelledCount     int64 `json:"cancelled_count"`
}

// ListFilteredLogs applies authorization and filters before pagination and totals.
func ListFilteredLogs(ctx context.Context, filters LogFilters, page, pageSize int64) ([]Log, int64, LogPageSummary, error) {
	page = max(1, min(page, 1000000000))
	pageSize = max(1, min(pageSize, 100))
	rows := []Log{}
	var total int64
	var summary LogPageSummary
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := func() *gorm.DB {
			q := tx.Model(&Log{})
			if filters.UserID > 0 {
				q = q.Where("user_id = ?", filters.UserID)
			}
			if filters.Model != "" {
				q = q.Where("model = ? OR requested_model = ?", filters.Model, filters.Model)
			}
			if filters.Status > 0 {
				q = q.Where("status = ?", filters.Status)
			}
			if filters.From > 0 {
				q = q.Where("created_at >= ?", filters.From)
			}
			if filters.Until > 0 {
				q = q.Where("created_at < ?", filters.Until)
			}
			if filters.Query != "" {
				term := "%" + strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(strings.ToLower(filters.Query)) + "%"
				condition := "(LOWER(model) LIKE ? ESCAPE '!' OR LOWER(requested_model) LIKE ? ESCAPE '!' OR LOWER(request_id) LIKE ? ESCAPE '!' OR LOWER(error_code) LIKE ? ESCAPE '!')"
				args := []any{term, term, term, term}
				if id, err := strconv.ParseInt(strings.TrimPrefix(filters.Query, "log_"), 10, 64); err == nil && id > 0 {
					condition = "(" + condition + " OR id = ?)"
					args = append(args, id)
				}
				q = q.Where(condition, args...)
			}
			return q
		}
		if err := query().Count(&total).Error; err != nil {
			return err
		}
		if err := query().Select(`
			COALESCE(SUM(CASE WHEN request_observed = TRUE THEN 1 ELSE 0 END), 0) AS observed_requests,
			COALESCE(SUM(CASE WHEN request_observed = TRUE AND admission_rejected = TRUE THEN 1 ELSE 0 END), 0) AS rejected_requests,
			COALESCE(SUM(CASE WHEN request_observed = TRUE THEN upstream_attempts ELSE 0 END), 0) AS upstream_attempts,
			COALESCE(SUM(CASE WHEN request_observed = FALSE THEN 1 ELSE 0 END), 0) AS historical_requests,
			COALESCE(SUM(CASE WHEN status = 1 THEN 1 ELSE 0 END), 0) AS success_count,
			COALESCE(SUM(CASE WHEN status = 2 THEN 1 ELSE 0 END), 0) AS failed_count,
			COALESCE(SUM(CASE WHEN status = 3 THEN 1 ELSE 0 END), 0) AS cancelled_count`).Scan(&summary).Error; err != nil {
			return err
		}
		return query().Order("id DESC").Limit(int(pageSize)).Offset(int((page - 1) * pageSize)).Find(&rows).Error
	})
	return rows, total, summary, err
}

// FinalizeRequestLog enriches the billing row without changing its monetary facts.
// Requests that never reached settlement receive a separate, uncharged row.
func FinalizeRequestLog(entry *Log) error {
	var existing Log
	err := DB.Where("request_id = ?", entry.RequestID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return InsertLog(entry)
	}
	if err != nil {
		return err
	}
	status := entry.Status
	if existing.Status != 1 && status == 1 {
		status = existing.Status
	}
	if status != 1 && entry.ErrorCode == "" {
		entry.FailureStage, entry.ErrorCode, entry.ErrorMessage = "response", "request_failed", "Request did not complete."
	}
	return DB.Model(&Log{}).Where("id = ?", existing.ID).Updates(map[string]any{
		"routing_fallback": existing.RoutingFallback || entry.RoutingFallback,
		"request_observed": true, "admission_rejected": entry.AdmissionRejected,
		"upstream_attempts": entry.UpstreamAttempts, "http_status": entry.HTTPStatus,
		"failure_stage": entry.FailureStage, "error_code": entry.ErrorCode,
		"error_message": entry.ErrorMessage, "request_path": entry.RequestPath, "status": status,
	}).Error
}

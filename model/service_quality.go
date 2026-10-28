package model

import (
	"fmt"
	"time"
)

// ServiceQualityMetric is a per-model quality summary computed from the
// Log table. It mirrors the headline SLA indicators from the resource
// project's Metric15m / ModelMetric15m models, simplified to what the
// StarHub Log table can provide.
type ServiceQualityMetric struct {
	Model         string  `json:"model"`
	TotalRequests int64   `json:"total_requests"`
	SuccessCount  int64   `json:"success_count"`
	FailedCount   int64   `json:"failed_count"`
	SuccessRate   float64 `json:"success_rate"` // 0.0–1.0
	AvgDurationMs int64   `json:"avg_duration_ms"`
	P95DurationMs int64   `json:"p95_duration_ms"`
	TotalQuota    int64   `json:"total_quota"`
	ErrorRate     float64 `json:"error_rate"` // 0.0–1.0
}

// ServiceQualityDailyMetric is one day's quality summary for a single
// model (or the overall aggregate when Model is empty).
type ServiceQualityDailyMetric struct {
	Date          string  `json:"date"` // "2006-01-02"
	Model         string  `json:"model"`
	TotalRequests int64   `json:"total_requests"`
	SuccessCount  int64   `json:"success_count"`
	FailedCount   int64   `json:"failed_count"`
	SuccessRate   float64 `json:"success_rate"`
	AvgDurationMs int64   `json:"avg_duration_ms"`
	P95DurationMs int64   `json:"p95_duration_ms"`
}

// ServiceQualitySummary is the top-level response: overall metrics plus
// per-model breakdown and optional per-day trends.
type ServiceQualitySummary struct {
	WindowHours int                         `json:"window_hours"`
	Overall     ServiceQualityMetric        `json:"overall"`
	ByModel     []ServiceQualityMetric      `json:"by_model"`
	ByDay       []ServiceQualityDailyMetric `json:"by_day"`
}

// SummarizeServiceQuality aggregates the Log table into per-model
// quality metrics for the given time window. When userID > 0, only
// that user's logs are included; 0 means all (admin view).
// An optional model filter applies to both totals and daily trends.
func SummarizeServiceQuality(userID int64, windowHours int, modelFilter ...string) (*ServiceQualitySummary, error) {
	if windowHours < 1 {
		windowHours = 24
	}
	if windowHours > 720 {
		windowHours = 720 // 30 days max
	}
	since := time.Now().UTC().Add(-time.Duration(windowHours) * time.Hour).Unix()

	baseWhere := "created_at >= ?"
	args := []any{since}
	if userID > 0 {
		baseWhere += " AND user_id = ?"
		args = append(args, userID)
	}
	if len(modelFilter) > 0 && modelFilter[0] != "" {
		baseWhere += " AND model = ?"
		args = append(args, modelFilter[0])
	}

	// Aggregate per model.
	type row struct {
		Model       string
		Total       int64
		Success     int64
		Failed      int64
		AvgDuration float64
		TotalQuota  int64
	}
	var rows []row
	err := DB.Model(&Log{}).
		Select("model AS model, COUNT(*) AS total, "+
			"SUM(CASE WHEN status = 1 THEN 1 ELSE 0 END) AS success, "+
			"SUM(CASE WHEN status = 2 THEN 1 ELSE 0 END) AS failed, "+
			"COALESCE(AVG(duration_ms), 0) AS avg_duration, "+
			"COALESCE(SUM(quota), 0) AS total_quota").
		Where(baseWhere, args...).
		Group("model").
		Order("total DESC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	percentiles, err := serviceQualityP95(baseWhere, args...)
	if err != nil {
		return nil, err
	}

	byModel := make([]ServiceQualityMetric, 0, len(rows))
	var overallTotal, overallSuccess, overallFailed, overallQuota int64
	var overallDurationSum float64
	for _, r := range rows {
		successRate := 0.0
		if r.Total > 0 {
			successRate = float64(r.Success) / float64(r.Total)
		}
		metric := ServiceQualityMetric{
			Model:         r.Model,
			TotalRequests: r.Total,
			SuccessCount:  r.Success,
			FailedCount:   r.Failed,
			SuccessRate:   successRate,
			AvgDurationMs: int64(r.AvgDuration),
			P95DurationMs: percentiles.ByModel[r.Model],
			TotalQuota:    r.TotalQuota,
			ErrorRate:     1.0 - successRate,
		}
		byModel = append(byModel, metric)
		overallTotal += r.Total
		overallSuccess += r.Success
		overallFailed += r.Failed
		overallQuota += r.TotalQuota
		overallDurationSum += r.AvgDuration * float64(r.Total)
	}

	overall := ServiceQualityMetric{
		TotalRequests: overallTotal,
		SuccessCount:  overallSuccess,
		FailedCount:   overallFailed,
		TotalQuota:    overallQuota,
		P95DurationMs: percentiles.Overall,
	}
	if overallTotal > 0 {
		overall.SuccessRate = float64(overallSuccess) / float64(overallTotal)
		overall.ErrorRate = 1.0 - overall.SuccessRate
		overall.AvgDurationMs = int64(overallDurationSum / float64(overallTotal))
	}

	// Daily breakdown for trend charts.
	type dayRow struct {
		DateStr string
		Total   int64
		Success int64
		Failed  int64
		AvgDur  float64
	}
	var dayRows []dayRow
	err = DB.Model(&Log{}).
		Select(fmt.Sprintf(
			"strftime('%%Y-%%m-%%d', datetime(created_at, 'unixepoch')) AS date_str, "+
				"COUNT(*) AS total, "+
				"SUM(CASE WHEN status = 1 THEN 1 ELSE 0 END) AS success, "+
				"SUM(CASE WHEN status = 2 THEN 1 ELSE 0 END) AS failed, "+
				"COALESCE(AVG(duration_ms), 0) AS avg_dur")).
		Where(baseWhere, args...).
		Group("date_str").
		Order("date_str ASC").
		Scan(&dayRows).Error
	if err != nil {
		return nil, err
	}
	byDay := make([]ServiceQualityDailyMetric, 0, len(dayRows))
	for _, dr := range dayRows {
		sr := 0.0
		if dr.Total > 0 {
			sr = float64(dr.Success) / float64(dr.Total)
		}
		byDay = append(byDay, ServiceQualityDailyMetric{
			Date:          dr.DateStr,
			TotalRequests: dr.Total,
			SuccessCount:  dr.Success,
			FailedCount:   dr.Failed,
			SuccessRate:   sr,
			AvgDurationMs: int64(dr.AvgDur),
			P95DurationMs: percentiles.ByDay[dr.DateStr],
		})
	}

	return &ServiceQualitySummary{
		WindowHours: windowHours,
		Overall:     overall,
		ByModel:     byModel,
		ByDay:       byDay,
	}, nil
}

// serviceQualityP95 uses the nearest rank ceil(0.95 * n). Non-positive
// durations have no measured latency and do not participate in the rank.
func serviceQualityP95(where string, args ...any) (serviceQualityPercentiles, error) {
	var ranks []struct {
		Scope      int
		MetricKey  string
		DurationMs int64
	}
	err := DB.Raw(`WITH samples AS (
		SELECT model, duration_ms,
			strftime('%Y-%m-%d', created_at, 'unixepoch') AS day
		FROM logs WHERE `+where+` AND duration_ms > 0
	), scopes AS (
		SELECT 0 AS scope, model AS metric_key, duration_ms FROM samples
		UNION ALL
		SELECT 1 AS scope, '' AS metric_key, duration_ms FROM samples
		UNION ALL
		SELECT 2 AS scope, day AS metric_key, duration_ms FROM samples
	), ranked AS (
		SELECT scope, metric_key, duration_ms,
			ROW_NUMBER() OVER (PARTITION BY scope, metric_key ORDER BY duration_ms) AS sample_rank,
			COUNT(*) OVER (PARTITION BY scope, metric_key) AS sample_count
		FROM scopes
	)
	SELECT scope, metric_key, duration_ms FROM ranked
	WHERE sample_rank = (sample_count * 95 + 99) / 100`, args...).Scan(&ranks).Error
	if err != nil {
		return serviceQualityPercentiles{}, err
	}
	result := serviceQualityPercentiles{
		ByModel: make(map[string]int64),
		ByDay:   make(map[string]int64),
	}
	for _, rank := range ranks {
		switch rank.Scope {
		case 0:
			result.ByModel[rank.MetricKey] = rank.DurationMs
		case 1:
			result.Overall = rank.DurationMs
		case 2:
			result.ByDay[rank.MetricKey] = rank.DurationMs
		}
	}
	return result, nil
}

type serviceQualityPercentiles struct {
	ByModel map[string]int64
	ByDay   map[string]int64
	Overall int64
}

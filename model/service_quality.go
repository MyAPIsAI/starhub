package model

import "time"

// ServiceQualityMetric is a per-model quality summary computed from the
// Log table. It mirrors the headline SLA indicators from the resource
// project's Metric15m / ModelMetric15m models, simplified to what the
// StarHub Log table can provide.
type ServiceQualityMetric struct {
	Model            string  `json:"model"`
	TotalRequests    int64   `json:"total_requests"`
	SuccessCount     int64   `json:"success_count"`
	FailedCount      int64   `json:"failed_count"`
	SuccessRate      float64 `json:"success_rate"`       // 0.0–1.0
	AvgDurationMs    int64   `json:"avg_duration_ms"`
	P95DurationMs    int64   `json:"p95_duration_ms"`
	TotalQuota       int64   `json:"total_quota"`
	ErrorRate        float64 `json:"error_rate"`         // 0.0–1.0
}

// ServiceQualitySummary is the top-level response: overall metrics plus
// per-model breakdown.
type ServiceQualitySummary struct {
	WindowHours int                     `json:"window_hours"`
	Overall     ServiceQualityMetric    `json:"overall"`
	ByModel     []ServiceQualityMetric  `json:"by_model"`
}

// SummarizeServiceQuality aggregates the Log table into per-model
// quality metrics for the given time window. When userID > 0, only
// that user's logs are included; 0 means all (admin view).
func SummarizeServiceQuality(userID int64, windowHours int) (*ServiceQualitySummary, error) {
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

	// Aggregate per model.
	type row struct {
		Model         string
		Total         int64
		Success       int64
		Failed        int64
		AvgDuration   int64
		TotalQuota    int64
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

	byModel := make([]ServiceQualityMetric, 0, len(rows))
	var overallTotal, overallSuccess, overallFailed, overallQuota int64
	var overallDurationSum int64
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
			TotalQuota:    r.TotalQuota,
			ErrorRate:     1.0 - successRate,
		}
		byModel = append(byModel, metric)
		overallTotal += r.Total
		overallSuccess += r.Success
		overallFailed += r.Failed
		overallQuota += r.TotalQuota
		overallDurationSum += r.AvgDuration * r.Total
	}

	overall := ServiceQualityMetric{
		TotalRequests: overallTotal,
		SuccessCount:  overallSuccess,
		FailedCount:   overallFailed,
		TotalQuota:    overallQuota,
	}
	if overallTotal > 0 {
		overall.SuccessRate = float64(overallSuccess) / float64(overallTotal)
		overall.ErrorRate = 1.0 - overall.SuccessRate
		overall.AvgDurationMs = overallDurationSum / overallTotal
	}

	return &ServiceQualitySummary{
		WindowHours: windowHours,
		Overall:     overall,
		ByModel:     byModel,
	}, nil
}

package model

import "context"

// RequestLogSummary counts persisted calls across all users and retained history.
// FailedCount includes cancellations, matching service-quality summaries.
type RequestLogSummary struct {
	ObservedRequests   int64 `json:"observed_requests"`
	RejectedRequests   int64 `json:"rejected_requests"`
	UpstreamAttempts   int64 `json:"upstream_attempts"`
	HistoricalRequests int64 `json:"historical_requests"`
	TotalRequests      int64 `json:"total_requests"`
	SuccessCount       int64 `json:"success_count"`
	FailedCount        int64 `json:"failed_count"`
}

func SummarizeRequestLogs(ctx context.Context) (RequestLogSummary, error) {
	var summary RequestLogSummary
	err := DB.WithContext(ctx).Model(&Log{}).
		Select("COUNT(*) AS total_requests, " +
			"COALESCE(SUM(CASE WHEN request_observed = TRUE THEN 1 ELSE 0 END), 0) AS observed_requests, " +
			"COALESCE(SUM(CASE WHEN request_observed = TRUE AND admission_rejected = TRUE THEN 1 ELSE 0 END), 0) AS rejected_requests, " +
			"COALESCE(SUM(CASE WHEN request_observed = TRUE THEN upstream_attempts ELSE 0 END), 0) AS upstream_attempts, " +
			"COALESCE(SUM(CASE WHEN request_observed = FALSE THEN 1 ELSE 0 END), 0) AS historical_requests, " +
			"COALESCE(SUM(CASE WHEN status = 1 THEN 1 ELSE 0 END), 0) AS success_count, " +
			"COALESCE(SUM(CASE WHEN status IN (2, 3) THEN 1 ELSE 0 END), 0) AS failed_count").
		Scan(&summary).Error
	return summary, err
}

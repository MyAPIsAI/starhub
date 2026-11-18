package controller

import (
	"net/http"
	"time"

	"starhub/common"
	"starhub/metric"
	"starhub/model"
	"starhub/service"
)

// GetMetrics serves GET /api/metrics: the relay observability surface
// (process-local counters and channel windows) plus persisted usage totals.
// Admin only: the data spans every user and channel.
func GetMetrics(w http.ResponseWriter, r *http.Request) {
	usage, err := model.SummarizeRequestLogs(r.Context())
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, "request statistics unavailable")
		return
	}
	common.ApiSuccess(w, struct {
		metric.Snapshot
		Usage     model.RequestLogSummary           `json:"usage"`
		Telemetry service.ProductionTelemetryStatus `json:"telemetry"`
	}{metric.TakeSnapshot(), usage, service.ProductionTelemetryHealth(time.Now())})
}

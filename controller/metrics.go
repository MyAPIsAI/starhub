package controller

import (
	"net/http"

	"starhub/common"
	"starhub/metric"
	"starhub/service"
	"time"
)

// GetMetrics serves GET /api/metrics: the relay observability surface
// (request counters + per-channel rolling TTFT / success rate). Admin
// only — the data spans every user and channel.
func GetMetrics(w http.ResponseWriter, r *http.Request) {
	common.ApiSuccess(w, struct {
		metric.Snapshot
		Telemetry service.ProductionTelemetryStatus `json:"telemetry"`
	}{metric.TakeSnapshot(), service.ProductionTelemetryHealth(time.Now())})
}

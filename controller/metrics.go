package controller

import (
	"net/http"

	"starhub/common"
	"starhub/metric"
)

// GetMetrics serves GET /api/metrics: the relay observability surface
// (request counters + per-channel rolling TTFT / success rate). Admin
// only — the data spans every user and channel.
func GetMetrics(w http.ResponseWriter, r *http.Request) {
	common.ApiSuccess(w, metric.TakeSnapshot())
}

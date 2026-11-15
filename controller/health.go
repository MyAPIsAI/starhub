package controller

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"

	"starhub/common"
	"starhub/model"
	"starhub/setting"
)

var draining atomic.Bool

func SetDraining(value bool) { draining.Store(value) }

func LivenessCheck(w http.ResponseWriter, r *http.Request) {
	common.ApiSuccess(w, map[string]any{"status": "alive"})
}

// ReadinessCheck includes the same initial snapshot gate as the relay.
func ReadinessCheck(w http.ResponseWriter, r *http.Request) {
	if draining.Load() {
		common.ApiError(w, http.StatusServiceUnavailable, "server is draining")
		return
	}
	if setting.IsCommercialMode() {
		if _, _, ready := model.RoutingSnapshotStatus(); !ready {
			common.ApiError(w, http.StatusServiceUnavailable, "waiting for resource routing snapshot")
			return
		}
	}
	HealthCheck(w, r)
}

// HealthCheck handles GET /api/health.
// Returns 200 with DB connectivity status. Used by Docker HEALTHCHECK
// and load balancers. Does not require authentication.
func HealthCheck(w http.ResponseWriter, r *http.Request) {
	if model.DB == nil {
		common.ApiError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	// Verify DB is reachable by issuing a trivial query.
	var one int
	err := model.DB.WithContext(ctx).Raw("SELECT 1").Scan(&one).Error
	if err != nil || one != 1 {
		common.ApiError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	common.ApiSuccess(w, map[string]any{
		"status":   "ok",
		"version":  common.Version,
		"database": "connected",
	})
}

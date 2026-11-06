package controller

import (
	"net/http"

	"starhub/common"
	"starhub/model"
)

// HealthCheck handles GET /api/health.
// Returns 200 with DB connectivity status. Used by Docker HEALTHCHECK
// and load balancers. Does not require authentication.
func HealthCheck(w http.ResponseWriter, r *http.Request) {
	// Verify DB is reachable by issuing a trivial query.
	var one int
	err := model.DB.Raw("SELECT 1").Scan(&one).Error
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

package middleware

import (
	"net/http"
	"strings"
	"time"

	"starhub/common"
	"starhub/model"
	"starhub/setting"
)

// ResourcePlatformAuth is a dedicated service identity, separate from customer
// API keys and browser sessions. Rotation and revocation are checked per request.
func ResourcePlatformAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization := r.Header.Get("Authorization")
		if !strings.HasPrefix(authorization, "Bearer ") || !model.ResourceCredentialValid(strings.TrimPrefix(authorization, "Bearer "), time.Now()) {
			common.ApiError(w, http.StatusUnauthorized, "invalid resource service credential")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Commercial traffic waits until platform denials are available for this process.
func ResourceStartupGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if setting.IsCommercialMode() && (strings.HasPrefix(r.URL.Path, "/v1/") || strings.HasPrefix(r.URL.Path, "/v1beta/")) {
			if _, _, ready := model.RoutingSnapshotStatus(); !ready {
				SetRequestFailure(r.Context(), "routing", "routing_snapshot_unavailable", "Waiting for a resource routing snapshot.")
				w.Header().Set("Retry-After", "5")
				common.ApiError(w, http.StatusServiceUnavailable, "waiting for resource routing snapshot")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

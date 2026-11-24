package middleware

import (
	"net/http"
	"strings"
	"time"

	"starhub/common"
	"starhub/model"
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

// Missing snapshots are handled by authorized ordinary-channel fallback.
func ResourceStartupGate(next http.Handler) http.Handler {
	return next
}

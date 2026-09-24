package middleware

import (
	"net/http"
	"strings"

	"starhub/common"
	"starhub/config"
)

// ManagementSecurity applies only to management traffic, never streaming
// relay responses. Non-browser Bearer clients need no Origin header.
func ManagementSecurity(next http.Handler) http.Handler {
	protection := http.NewCrossOriginProtection()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			if err := protection.Check(r); err != nil {
				common.ApiError(w, http.StatusForbidden, "cross-origin management request rejected")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// SecureSessionCookie supports TLS termination without trusting forwarded
// headers from arbitrary clients. Operators explicitly enable it at a proxy.
func SecureSessionCookie(r *http.Request) bool {
	return r.TLS != nil || config.Current().CookieSecure || config.ForceSecureCookie()
}

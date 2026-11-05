package middleware

import (
	"net/http"
	"starhub/common"
	"time"
)

var staffLoginLimiter = newFixedWindowLimiter(10)

func StaffLoginRateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !staffLoginLimiter.allow(common.ClientIP(r), time.Now()) {
			writeRateLimited(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

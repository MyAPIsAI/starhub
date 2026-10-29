package middleware

import (
	"net/http"
	"strconv"
	"time"

	"starhub/common"
)

const PasswordVerificationLimitPerMinute = 10

var passwordVerificationLimiter = newFixedWindowLimiter(PasswordVerificationLimitPerMinute)

// PasswordVerificationRateLimit runs after UserAuth. The owner-wide bucket
// cannot be bypassed by changing token IDs or moving between client addresses.
func PasswordVerificationRateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, _ := r.Context().Value(common.CtxUserID).(int64)
		if userID <= 0 {
			common.ApiError(w, http.StatusUnauthorized, "login required")
			return
		}
		if !passwordVerificationLimiter.allow(strconv.FormatInt(userID, 10), time.Now()) {
			writeRateLimited(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

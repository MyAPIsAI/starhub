package middleware

import (
	"net/http"
	"strings"

	"starhub/common"
	"starhub/service/authz"
)

// FinanceAuth admits authenticated staff only for read-only finance routes.
// Every request reloads the role, so revocation takes effect immediately.
func FinanceAuth(next http.Handler) http.Handler {
	return UserAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role, _ := r.Context().Value(common.CtxRole).(int)
		path := strings.TrimSuffix(r.URL.Path, "/")
		financePath := path == "/api/finance" || strings.HasPrefix(path, "/api/finance/")
		if !financePath || (r.Method != http.MethodGet && r.Method != http.MethodHead) || !authz.Allow(role, authz.ResourceFinance, authz.ActionRead) {
			common.ApiError(w, http.StatusForbidden, "read-only finance privilege required")
			return
		}
		next.ServeHTTP(w, r)
	}))
}

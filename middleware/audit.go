package middleware

import (
	"net/http"

	"starhub/common"
	"starhub/model"
)

// Audit logs admin write operations to the audit_logs table. It wraps the
// AdminAuth chain: every POST/PUT/DELETE on /api/ endpoints that reaches an
// admin handler leaves one row regardless of the response status. GET
// requests and relay traffic are not audited (read-only).
func Audit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if shouldAudit(r) {
			uid, _ := r.Context().Value(common.CtxUserID).(int64)
			entry := &model.AuditLog{
				UserID: uid,
				Method: r.Method,
				Path:   r.URL.Path,
				Status: rec.status,
				IP:     clientIP(r),
			}
			// Best-effort: an audit failure must not change the response.
			_ = model.InsertAuditLog(entry)
		}
	})
}

// shouldAudit reports whether the request is an admin write on a management
// API path (relay traffic is excluded).
func shouldAudit(r *http.Request) bool {
	if r.Method == http.MethodGet {
		return false
	}
	path := r.URL.Path
	if len(path) >= 4 && path[:4] == "/api" {
		return true
	}
	return false
}

// statusRecorder captures the response status code for the audit log.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

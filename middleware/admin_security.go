package middleware

import (
	"net/http"
	"starhub/common"
	"starhub/model"
)

func allowStaffSecuritySetup(r *http.Request) bool {
	switch r.Method + " " + r.URL.Path {
	case "GET /api/user/self", "PUT /api/user/self", "GET /api/user/profile-state",
		"PUT /api/user/profile-state", "GET /api/user/security-state", "GET /api/user/session-state",
		"POST /api/user/logout", "POST /api/user/logout-current", "GET /api/user/2fa/status",
		"POST /api/user/2fa/setup", "POST /api/user/2fa/enable":
		return true
	}
	return false
}

func staffSecurityReady(w http.ResponseWriter, r *http.Request, user *model.User) bool {
	if !common.IsStaffRole(user.Role) || allowStaffSecuritySetup(r) {
		return true
	}
	if user.AdminPasswordChangedAt == 0 || !Sessions.StaffVerified(SessionToken(r), user) {
		common.ApiError(w, http.StatusForbidden, "staff security setup required: change the initial password, enable 2FA, and sign in again")
		return false
	}
	return true
}

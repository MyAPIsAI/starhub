package middleware

import (
	"net/http"
	"os"
	"strings"
	"time"

	"starhub/common"
	"starhub/model"
)

func allowStaffSecuritySetup(r *http.Request) bool {
	switch r.Method + " " + r.URL.Path {
	case "GET /api/user/self", "PUT /api/user/self", "GET /api/user/profile-state",
		"PUT /api/user/profile-state", "GET /api/user/security-state", "GET /api/user/session-state",
		"POST /api/user/logout", "POST /api/user/logout-current", "GET /api/user/2fa/status",
		"POST /api/user/2fa/setup", "POST /api/user/2fa/enable", "POST /api/user/2fa/skip":
		return true
	}
	return false
}

func staffSecurityReady(w http.ResponseWriter, r *http.Request, user *model.User) bool {
	if !common.IsStaffRole(user.Role) || allowStaffSecuritySetup(r) {
		return true
	}
	if StaffSecurityRequired(SessionToken(r), user) {
		common.ApiError(w, http.StatusForbidden, "staff security setup required: change the initial password, enable 2FA, and sign in again")
		return false
	}
	return true
}

// CanSkipStaffTwoFA permits deferring enrollment in development only. Existing
// factors and initial password changes must still follow their normal checks.
func CanSkipStaffTwoFA(user *model.User) bool {
	mode := strings.TrimSpace(os.Getenv("STARHUB_ENV"))
	if (mode != "" && mode != "development") || !common.IsStaffRole(user.Role) || user.Status != common.StatusEnabled || user.AdminPasswordChangedAt == 0 {
		return false
	}
	factor, err := model.GetTwoFA(user.ID)
	return err == model.Err2FANotEnabled || (err == nil && !factor.IsEnabled)
}

// SkipStaffTwoFA binds the development exception to one active session and the
// current account security state. It never creates second-factor evidence.
func (s *SessionStore) SkipStaffTwoFA(token string, user *model.User) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	sd := s.data[token]
	if !activeSession(sd, time.Now()) || sd.userID != user.ID || sd.securityVersion != user.DeactivationVersion || sd.factorRole != user.Role || sd.factorSecret != "" {
		return false
	}
	fresh, err := model.GetUserByID(user.ID)
	if err != nil || fresh.Password != user.Password || fresh.Role != user.Role || fresh.DeactivationVersion != user.DeactivationVersion || !CanSkipStaffTwoFA(fresh) {
		return false
	}
	sd.twoFASkipped = true
	sd.factorPassword = fresh.Password
	sd.factorRole = fresh.Role
	return true
}

func (s *SessionStore) staffTwoFASkipped(token string, user *model.User) bool {
	s.mu.RLock()
	sd := s.data[token]
	valid := activeSession(sd, time.Now()) && sd.userID == user.ID && sd.securityVersion == user.DeactivationVersion && sd.twoFASkipped && sd.factorPassword == user.Password && sd.factorRole == user.Role
	s.mu.RUnlock()
	return valid && CanSkipStaffTwoFA(user)
}

// StaffSecurityRequired is shared by API authorization and session restoration.
func StaffSecurityRequired(token string, user *model.User) bool {
	return common.IsStaffRole(user.Role) && (user.AdminPasswordChangedAt == 0 || (!Sessions.StaffVerified(token, user) && !Sessions.staffTwoFASkipped(token, user)))
}

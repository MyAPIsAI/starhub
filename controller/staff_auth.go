package controller

import (
	"fmt"
	"net/http"
	"net/url"
	"time"

	"starhub/common"
	"starhub/config"
	"starhub/middleware"
	"starhub/model"
)

func staffSessionCookie(w http.ResponseWriter, r *http.Request, token string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{Name: "staff_session", Value: token, Path: "/", HttpOnly: true, Secure: middleware.SecureSessionCookie(r), SameSite: http.SameSiteStrictMode, MaxAge: int(ttl.Seconds())})
}

func pendingStaff(w http.ResponseWriter, r *http.Request) (*model.User, middleware.StaffSessionState, bool) {
	w.Header().Set("Cache-Control", "private, no-store")
	state, ok := middleware.Sessions.StaffState(middleware.StaffSessionToken(r))
	if !ok {
		common.ApiError(w, 401, "staff login required")
		return nil, state, false
	}
	user, err := model.GetStaffByID(state.UserID)
	if err != nil || user.AccountKind != "staff" || !common.IsStaffRole(user.Role) || user.Status != common.StatusEnabled || user.SecurityVersion != state.Version {
		common.ApiError(w, 401, "staff session expired")
		return nil, state, false
	}
	return user, state, true
}

func staffAuthState(user *model.User, state middleware.StaffSessionState) map[string]any {
	stage := "verify"
	if user.MustChangePassword {
		stage = "change_password"
	} else if user.TOTPSecret == "" {
		stage = "enroll"
	} else if state.Verified {
		stage = "ready"
	}
	return map[string]any{"stage": stage, "id": user.ID, "username": user.Username, "role": user.Role, "account_kind": "staff"}
}

func staffAuthAudit(user *model.User, r *http.Request, status int) {
	actions := map[string]string{"/api/admin/auth/login": "staff_password_authenticated", "/api/admin/auth/password": "staff_password_changed", "/api/admin/auth/verify": "staff_2fa_verified"}
	action := actions[r.URL.Path]
	if status >= 400 {
		action += "_rejected"
	}
	_ = model.InsertAuditLog(&model.AuditLog{UserID: user.ID, Username: user.Username, ActorRole: user.Role, Action: action, ObjectType: "staff", ObjectID: fmt.Sprint(user.ID), Method: r.Method, Path: r.URL.Path, Status: status, IP: common.ClientIP(r)})
}

func StaffLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeJSONBody(w, r, &input) {
		return
	}
	user, err := model.GetStaffByUsername(input.Username)
	if err != nil || user.AccountKind != "staff" || !common.IsStaffRole(user.Role) || user.Status != common.StatusEnabled || !common.VerifyPassword(input.Password, user.Password) {
		common.ApiError(w, 401, "invalid staff credentials")
		return
	}
	if user.OTPLockedUntil > time.Now().Unix() {
		common.ApiError(w, 429, "staff verification temporarily locked")
		return
	}
	token := middleware.Sessions.CreateStaffConfirmed(user, false, 5*time.Minute)
	if token == "" {
		common.ApiError(w, 409, "staff account changed; sign in again")
		return
	}
	middleware.Sessions.Remove(middleware.StaffSessionToken(r))
	staffSessionCookie(w, r, token, 5*time.Minute)
	staffAuthAudit(user, r, 200)
	common.ApiSuccess(w, staffAuthState(user, middleware.StaffSessionState{}))
}

func GetStaffAuthState(w http.ResponseWriter, r *http.Request) {
	user, state, ok := pendingStaff(w, r)
	if !ok {
		return
	}
	common.ApiSuccess(w, staffAuthState(user, state))
}

func GetStaffSelf(w http.ResponseWriter, r *http.Request) {
	user, err := model.GetStaffByID(requestUserID(r))
	if err != nil {
		common.ApiError(w, 401, "staff session expired")
		return
	}
	common.ApiSuccess(w, publicUser(user))
}

func StaffChangePassword(w http.ResponseWriter, r *http.Request) {
	user, state, ok := pendingStaff(w, r)
	if !ok {
		return
	}
	var input struct {
		Current  string `json:"current_password"`
		Password string `json:"new_password"`
	}
	if !decodeJSONBody(w, r, &input) {
		return
	}
	if !user.MustChangePassword && !state.Verified {
		common.ApiError(w, 403, "complete 2FA before changing a bound account password")
		return
	}
	fresh, err := model.ChangeStaffPassword(user.ID, state.Version, input.Current, input.Password)
	if err != nil {
		staffAuthAudit(user, r, 403)
		common.ApiError(w, 403, err.Error())
		return
	}
	middleware.Sessions.RemoveAllExcept(user.ID, "")
	token := middleware.Sessions.CreateStaffConfirmed(fresh, false, 5*time.Minute)
	if token == "" {
		common.ApiError(w, 409, "staff account changed; sign in again")
		return
	}
	staffSessionCookie(w, r, token, 5*time.Minute)
	staffAuthAudit(fresh, r, 200)
	common.ApiSuccess(w, staffAuthState(fresh, middleware.StaffSessionState{}))
}

func StaffBeginEnrollment(w http.ResponseWriter, r *http.Request) {
	user, state, ok := pendingStaff(w, r)
	if !ok {
		return
	}
	if user.MustChangePassword || user.TOTPSecret != "" || state.Verified {
		common.ApiError(w, 403, "2FA enrollment is not available in this state")
		return
	}
	secret := state.EnrollmentSecret
	if secret == "" {
		var err error
		secret, err = model.NewTOTPSecret()
		if err != nil {
			common.ApiError(w, 500, "2FA setup unavailable")
			return
		}
	}
	if !middleware.Sessions.SetStaffEnrollment(middleware.StaffSessionToken(r), secret) {
		common.ApiError(w, 401, "staff session expired")
		return
	}
	state, _ = middleware.Sessions.StaffState(middleware.StaffSessionToken(r))
	secret = state.EnrollmentSecret
	values := url.Values{"secret": {secret}, "issuer": {"MyAPIs Staff"}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}
	common.ApiSuccess(w, map[string]any{"secret": secret, "uri": "otpauth://totp/" + url.PathEscape("MyAPIs Staff:"+user.Username) + "?" + values.Encode()})
}

func StaffVerifyOTP(w http.ResponseWriter, r *http.Request) {
	user, state, ok := pendingStaff(w, r)
	if !ok {
		return
	}
	var input struct {
		Code string `json:"code"`
	}
	if !decodeJSONBody(w, r, &input) {
		return
	}
	secret := ""
	if user.TOTPSecret == "" {
		secret = state.EnrollmentSecret
	}
	fresh, err := model.VerifyStaffOTP(user.ID, state.Version, input.Code, secret, time.Now())
	if err != nil {
		staffAuthAudit(user, r, 403)
		common.ApiError(w, 403, "invalid, expired or already used verification code")
		return
	}
	token := middleware.Sessions.CreateStaffConfirmed(fresh, true, 8*time.Hour)
	if token == "" {
		common.ApiError(w, 409, "staff account changed; sign in again")
		return
	}
	middleware.Sessions.Remove(middleware.StaffSessionToken(r))
	middleware.ClearOtherPortalSession(w, r, "session")
	staffSessionCookie(w, r, token, 8*time.Hour)
	staffAuthAudit(fresh, r, 200)
	common.ApiSuccess(w, staffAuthState(fresh, middleware.StaffSessionState{Verified: true}))
}

func StaffLogout(w http.ResponseWriter, r *http.Request) {
	middleware.Sessions.Remove(middleware.StaffSessionToken(r))
	http.SetCookie(w, &http.Cookie{Name: "staff_session", Value: "", Path: "/", HttpOnly: true, Secure: middleware.SecureSessionCookie(r), SameSite: http.SameSiteStrictMode, MaxAge: -1})
	common.ApiSuccess(w, nil)
}

func StaffInstallationState(w http.ResponseWriter, r *http.Request) {
	exists, err := model.StaffRootExists()
	if err != nil {
		common.ApiError(w, 500, "installation state unavailable")
		return
	}
	common.ApiSuccess(w, map[string]any{"available": config.StaffOpenSourceSetup() && !exists})
}

func InstallStaffRoot(w http.ResponseWriter, r *http.Request) {
	if !config.StaffOpenSourceSetup() {
		common.ApiError(w, 404, "staff installation unavailable")
		return
	}
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Token    string `json:"installation_token"`
	}
	if !decodeJSONBody(w, r, &input) {
		return
	}
	if !validUsername(input.Username) {
		common.ApiError(w, 400, "invalid username")
		return
	}
	if err := model.InitializeStaffRoot(input.Username, input.Password, input.Token, true); err != nil {
		common.ApiError(w, 403, "installation token is invalid, expired or already consumed")
		return
	}
	common.ApiSuccess(w, nil)
}

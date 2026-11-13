package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
	"starhub/pkg/localepolicy"
	"starhub/setting"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type registerRequest struct {
	Language         string `json:"language"`
	Username         string `json:"username"`
	Password         string `json:"password"`
	Email            string `json:"email"`
	AffCode          string `json:"aff_code"`
	AgreementVersion string `json:"agreement_version"`
}

func validUsername(name string) bool {
	if len(name) < 2 || len(name) > 24 {
		return false
	}
	for _, c := range name {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9',
			c == '_', c == '-', c == '.':
		default:
			return false
		}
	}
	return true
}

// validPassword checks that the password is at least 8 characters, contains
// both letters and digits, and is not a known weak password.
func validPassword(password string) bool {
	if len(password) < 8 || len(password) > 64 {
		return false
	}
	hasLetter := false
	hasDigit := false
	for _, c := range password {
		switch {
		case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'):
			hasLetter = true
		case c >= '0' && c <= '9':
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return false
	}
	return !isWeakPassword(password)
}

// weakPasswords is a small set of commonly used passwords that pass the
// letter+digit rule but should still be rejected.
var weakPasswords = map[string]bool{
	"password1": true, "abc12345": true, "qwerty12": true,
	"12345678": true, "11111111": true, "00000000": true,
	"abcd1234": true, "test1234": true, "admin123": true,
	"letmein1": true, "welcome1": true, "monkey12": true,
}

func isWeakPassword(password string) bool {
	return weakPasswords[strings.ToLower(password)]
}

func validEmail(email string) bool {
	if email == "" {
		return true
	}
	if len(email) < 5 || len(email) > 64 {
		return false
	}
	at := strings.Index(email, "@")
	return at > 0 && at < len(email)-1
}

func Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	region, ok := requireRegionFeature(w, "registration", "password")
	if !ok {
		return
	}
	if region.LegalVersion != "" && req.AgreementVersion != region.LegalVersion {
		common.ApiError(w, 409, "confirm the current regional legal version before registering")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if !validUsername(req.Username) {
		common.ApiError(w, http.StatusBadRequest,
			"username must be 2-24 characters of letters, digits, '_', '-', '.'")
		return
	}
	if !validPassword(req.Password) {
		common.ApiError(w, http.StatusBadRequest,
			"password must be 8-64 characters with at least one letter and one digit")
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	if !validEmail(req.Email) {
		common.ApiError(w, http.StatusBadRequest, "invalid email")
		return
	}
	if _, err := model.GetUserByUsername(req.Username); err == nil {
		common.ApiError(w, http.StatusConflict, "username already exists")
		return
	} else if err != model.ErrUserNotFound {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	hashed, err := common.HashPassword(req.Password)
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	user := &model.User{
		Language: localepolicy.Resolve(req.Language, r.Header.Get("Accept-Language"), region.DefaultLanguage, region.SupportedLanguageList()),
		Username: req.Username,
		Email:    req.Email,
		Password: hashed,
		Role:     common.RoleUser,
		Status:   common.StatusEnabled,
		Quota:    common.DefaultRegisterQuota,
		Group:    common.DefaultGroup,
		AffCode:  common.GenerateAffCode(),
	}
	// Resolve the inviter from the aff code (if provided). The inviter's
	// register bonus is credited atomically after the user is created.
	var inviterID int64
	if code := strings.TrimSpace(req.AffCode); code != "" {
		inviter, err := model.GetUserByAffCode(code)
		if err == nil && inviter.Status == common.StatusEnabled {
			inviterID = inviter.ID
			user.InviterID = inviterID
		}
	}
	if err := model.CreateRegisteredUser(user, req.AgreementVersion, region); err != nil {
		if errors.Is(err, model.ErrRegionRegistrationChanged) {
			common.ApiError(w, 409, err.Error())
			return
		}
		if errors.Is(err, model.ErrRegionLegalUnavailable) {
			common.ApiError(w, 503, err.Error())
			return
		}
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			common.ApiError(w, http.StatusConflict, "username already exists")
			return
		}
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Record agreement consent (best-effort: a failure does not roll back
	// the registration).
	// Credit the inviter's register bonus (best-effort: a failure must not
	// roll back the registration).
	if inviterID > 0 {
		if bonus := setting.AffRegisterBonusValue(); bonus > 0 {
			if err := model.IncreaseUserQuota(inviterID, bonus); err != nil {
				// Log only; the registration itself succeeded.
				_ = err
			}
		}
	}
	common.ApiSuccess(w, publicUser(user))
}

func Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	// Check login lockout before any database lookup.
	if locked, remaining := middleware.IsLocked(req.Username); locked {
		common.ApiError(w, http.StatusTooManyRequests,
			"account is temporarily locked due to repeated failures; try again in "+remaining.Round(time.Second).String())
		return
	}
	user, err := model.GetUserByUsername(req.Username)
	if err != nil || !common.VerifyPassword(req.Password, user.Password) {
		middleware.RecordLoginFailure(req.Username)
		common.ApiError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	if user.Status != common.StatusEnabled {
		common.ApiError(w, http.StatusForbidden, "user is disabled")
		return
	}
	if !common.IsStaffRole(user.Role) {
		if _, ok := requireRegionFeature(w, "password_login", "password"); !ok {
			return
		}
	}
	if beginTwoFactor(w, r, user) {
		return
	}
	middleware.RecordLoginSuccess(req.Username)
	const sessionTTL = 7 * 24 * time.Hour
	token := middleware.Sessions.CreateConfirmed(user, sessionTTL, clientIP(r), r.UserAgent())
	if token == "" {
		common.ApiError(w, http.StatusConflict, "account changed during login; retry sign-in")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   middleware.SecureSessionCookie(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
	common.ApiSuccess(w, map[string]any{
		"session": token,
		"user":    publicUser(user),
	})
}

func Logout(w http.ResponseWriter, r *http.Request) {
	middleware.Sessions.Remove(middleware.SessionToken(r))
	http.SetCookie(w, &http.Cookie{Name: "session", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: middleware.SecureSessionCookie(r), SameSite: http.SameSiteLaxMode})
	common.ApiSuccess(w, nil)
}

// DeactivateSelf requires a confirmed snapshot and never changes the cookie.
func DeactivateSelf(w http.ResponseWriter, r *http.Request) {
	deactivateOwnedAccount(w, r)
}

// ListSessions returns the current user's active sessions.
func ListSessions(w http.ResponseWriter, r *http.Request) {
	token := middleware.SessionToken(r)
	infos := middleware.Sessions.ListByUser(requestUserID(r), token)
	common.ApiSuccess(w, infos)
}

// TerminateOtherSessions removes all sessions for the current user
// except the one making the request.
func TerminateOtherSessions(w http.ResponseWriter, r *http.Request) {
	owner, ok := sessionTerminationOwner(w, r)
	if !ok {
		return
	}
	count := middleware.Sessions.RemoveAllExcept(owner, middleware.SessionToken(r))
	common.ApiSuccess(w, map[string]any{"terminated": count, "user_id": owner})
}

func GetSelf(w http.ResponseWriter, r *http.Request) {
	user, err := model.GetUserByID(requestUserID(r))
	if err != nil {
		common.ApiError(w, http.StatusNotFound, "user not found")
		return
	}
	requestCount, err := model.CountLogs(user.ID)
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	data := publicUser(user)
	data["request_count"] = requestCount
	data["admin_security_required"] = common.IsStaffRole(user.Role) && (user.AdminPasswordChangedAt == 0 || !middleware.Sessions.StaffVerified(middleware.SessionToken(r), user))
	common.ApiSuccess(w, data)
}

func UserUsage(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	days, _ := strconv.ParseInt(q.Get("days"), 10, 64)
	if days < 1 {
		days = 7
	}
	if days > 90 {
		days = 90
	}
	userID := requestUserID(r)
	if raw := q.Get("user_id"); raw != "" && common.IsAdminRole(requestRole(r)) {
		if v, err := strconv.ParseInt(raw, 10, 64); err == nil && v >= 0 {
			userID = v
		}
	}
	tokenID, _ := strconv.ParseInt(q.Get("token_id"), 10, 64)
	since := time.Now().UTC().AddDate(0, 0, -int(days)+1).Truncate(24 * time.Hour).Unix()
	items, err := model.SummarizeLogsByDay(userID, since)
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.ApiSuccess(w, map[string]any{
		"days":  days,
		"items": items,
		"by_model": func() any {
			modelItems, err := model.SummarizeLogsByModel(userID, since)
			if err != nil {
				return []any{}
			}
			return modelItems
		}(),
		"by_token": func() any {
			tokenItems, err := model.SummarizeLogsByToken(userID, tokenID, since)
			if err != nil {
				return []any{}
			}
			return tokenItems
		}(),
	})
}

func ListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := model.ListUsers()
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	data := make([]map[string]any, 0, len(users))
	for i := range users {
		data = append(data, publicUser(&users[i]))
	}
	common.ApiSuccess(w, data)
}

type userUpdateRequest struct {
	ID            int64   `json:"id"`
	Quota         *int64  `json:"quota"`
	Status        *int    `json:"status"`
	Group         *string `json:"group"`
	Password      *string `json:"password"`
	ExpectedQuota *int64  `json:"expected_quota"`
	Reason        string  `json:"reason"`
}

func UpdateUser(w http.ResponseWriter, r *http.Request) {
	var req userUpdateRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.ID <= 0 {
		common.ApiError(w, http.StatusBadRequest, "user id is required")
		return
	}
	user, err := model.GetUserByID(req.ID)
	if err != nil {
		common.ApiError(w, http.StatusNotFound, "user not found")
		return
	}
	fields := map[string]any{}
	// Quota changes go through AdjustUserQuota so they write a LedgerEntry
	// with actor attribution and optimistic-concurrency guard. Other
	// fields (status, group, password) still use UpdateUserFieldsWithQuota.
	if req.Quota != nil {
		if *req.Quota < 0 {
			common.ApiError(w, http.StatusBadRequest, "quota must be >= 0")
			return
		}
		if req.ExpectedQuota == nil {
			common.ApiError(w, http.StatusBadRequest, "expected_quota is required when setting a balance")
			return
		}
		delta := *req.Quota - user.Quota
		if delta != 0 {
			reason := strings.TrimSpace(req.Reason)
			if reason == "" {
				common.ApiError(w, http.StatusBadRequest, "reason is required when adjusting quota")
				return
			}
			if _, err := model.AdjustUserQuota(req.ID, delta, *req.ExpectedQuota, requestUserID(r), reason); err != nil {
				if err == model.ErrAdjustmentConflict {
					common.ApiError(w, http.StatusConflict, "balance changed; reload and confirm again")
					return
				}
				common.ApiError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
	}
	if req.Status != nil {
		if req.ID == requestUserID(r) && *req.Status != common.StatusEnabled {
			common.ApiError(w, http.StatusBadRequest, "cannot disable the current account")
			return
		}
		if *req.Status != common.StatusEnabled && *req.Status != common.StatusDisabled {
			common.ApiError(w, http.StatusBadRequest, "invalid status")
			return
		}
		fields["status"] = *req.Status
	}
	if req.Group != nil {
		if !setting.ContainsGroupRatio(*req.Group) {
			common.ApiError(w, http.StatusBadRequest, "group has no ratio configured")
			return
		}
		fields["group"] = *req.Group
	}
	if req.Password != nil {
		if len(*req.Password) < 6 {
			common.ApiError(w, http.StatusBadRequest, "password must be at least 6 characters")
			return
		}
		hashed, err := common.HashPassword(*req.Password)
		if err != nil {
			common.ApiError(w, http.StatusInternalServerError, err.Error())
			return
		}
		fields["password"] = hashed
	}
	// When quota was adjusted via AdjustUserQuota, other field updates may
	// be empty (quota-only update). Skip the "nothing to update" guard in
	// that case, and pass nil expectedQuota to UpdateUserFieldsWithQuota so
	// it does not re-check the quota guard (already done by AdjustUserQuota).
	var fieldsExpectedQuota *int64
	if req.Quota != nil {
		fieldsExpectedQuota = nil
	} else {
		fieldsExpectedQuota = req.ExpectedQuota
	}
	if len(fields) == 0 && req.Quota == nil {
		common.ApiError(w, http.StatusBadRequest, "nothing to update")
		return
	}
	if len(fields) > 0 {
		if err := model.UpdateUserFieldsWithQuota(req.ID, fields, fieldsExpectedQuota); err != nil {
			if err == model.ErrQuotaConflict {
				common.ApiError(w, http.StatusConflict, "balance changed; reload and confirm again")
				return
			}
			common.ApiError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	// When the user is disabled, revoke all their sessions so active
	// sessions cannot continue to make relay calls.
	if req.Status != nil && *req.Status == common.StatusDisabled {
		middleware.Sessions.RemoveAllExcept(req.ID, "")
	}
	common.ApiSuccess(w, nil)
}

func publicUser(u *model.User) map[string]any {
	return map[string]any{
		"quota_nano": common.QuotaMoney(u.Quota, u.QuotaRemainderNano).String(), "used_quota_nano": common.QuotaMoney(u.UsedQuota, u.UsedQuotaRemainderNano).String(),
		"id":                             u.ID,
		"username":                       u.Username,
		"email":                          u.Email,
		"role":                           u.Role,
		"status":                         u.Status,
		"quota":                          u.Quota,
		"used_quota":                     u.UsedQuota,
		"group":                          u.Group,
		"aff_code":                       u.AffCode,
		"inviter_id":                     u.InviterID,
		"language":                       u.Language,
		"created_at":                     u.CreatedAt,
		"admin_password_change_required": common.IsStaffRole(u.Role) && u.AdminPasswordChangedAt == 0,
	}
}

// --- Self-service profile updates ---

type updateSelfRequest struct {
	CurrentPassword *string `json:"current_password"`
	NewPassword     *string `json:"new_password"`
	Email           *string `json:"email"`
	Language        *string `json:"language"`
}

// UpdateSelf lets the authenticated user change their own password and
// email. Password change requires the current password; email change
// does not (matches the demo UI which treats email as a profile field).
func UpdateSelf(w http.ResponseWriter, r *http.Request) {
	var req updateSelfRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	user, err := model.GetUserByID(requestUserID(r))
	if err != nil {
		common.ApiError(w, http.StatusNotFound, "user not found")
		return
	}
	fields := map[string]any{}
	if req.NewPassword != nil {
		if req.CurrentPassword == nil || !common.VerifyPassword(*req.CurrentPassword, user.Password) {
			common.ApiError(w, http.StatusForbidden, "current password is incorrect")
			return
		}
		pw := strings.TrimSpace(*req.NewPassword)
		if !validPassword(pw) {
			common.ApiError(w, http.StatusBadRequest,
				"password must be 8-64 characters with at least one letter and one digit")
			return
		}
		if common.IsStaffRole(user.Role) && common.VerifyPassword(pw, user.Password) {
			common.ApiError(w, http.StatusBadRequest, "new password must differ from the current password")
			return
		}
		hashed, err := common.HashPassword(pw)
		if err != nil {
			common.ApiError(w, http.StatusInternalServerError, err.Error())
			return
		}
		fields["password"] = hashed
		if common.IsStaffRole(user.Role) {
			fields["admin_password_changed_at"] = time.Now().Unix()
		}
	}
	if req.Email != nil {
		email := strings.TrimSpace(*req.Email)
		if !validEmail(email) {
			common.ApiError(w, http.StatusBadRequest, "invalid email")
			return
		}
		fields["email"] = email
	}
	if req.Language != nil {
		lang := strings.TrimSpace(*req.Language)
		if lang != "" && lang != "zh" && lang != "en" && lang != "ar" && lang != "ko" {
			common.ApiError(w, http.StatusBadRequest, "unsupported language; use zh, en, ar or ko")
			return
		}
		fields["language"] = lang
	}
	if len(fields) == 0 {
		common.ApiError(w, http.StatusBadRequest, "nothing to update")
		return
	}
	if err := model.UpdateUserFields(user.ID, fields); err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// When the password changes, revoke all other sessions so the user must
	// re-authenticate on other devices. The current session stays active.
	if _, ok := fields["password"]; ok {
		currentToken := middleware.SessionToken(r)
		middleware.Sessions.RemoveAllExcept(user.ID, currentToken)
	}
	// Return the updated public user so the caller can refresh state.
	updated, err := model.GetUserByID(user.ID)
	if err != nil {
		common.ApiSuccess(w, nil)
		return
	}
	common.ApiSuccess(w, publicUser(updated))
}

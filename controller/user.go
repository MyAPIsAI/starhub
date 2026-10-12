package controller

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
	"starhub/setting"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type registerRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email"`
	AffCode  string `json:"aff_code"`
}

func validUsername(name string) bool {
	if len(name) < 3 || len(name) > 64 {
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
	req.Username = strings.TrimSpace(req.Username)
	if !validUsername(req.Username) {
		common.ApiError(w, http.StatusBadRequest,
			"username must be 3-64 characters of letters, digits, '_', '-', '.'")
		return
	}
	if len(req.Password) < 6 || len(req.Password) > 64 {
		common.ApiError(w, http.StatusBadRequest, "password must be 6-64 characters")
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
	if err := model.CreateUser(user); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			common.ApiError(w, http.StatusConflict, "username already exists")
			return
		}
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
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
	user, err := model.GetUserByUsername(req.Username)
	if err != nil || !common.VerifyPassword(req.Password, user.Password) {
		common.ApiError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	if user.Status != common.StatusEnabled {
		common.ApiError(w, http.StatusForbidden, "user is disabled")
		return
	}
	const sessionTTL = 7 * 24 * time.Hour
	token := middleware.Sessions.Create(user.ID, sessionTTL)
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
	if raw := q.Get("user_id"); raw != "" && requestRole(r) >= common.RoleAdmin {
		if v, err := strconv.ParseInt(raw, 10, 64); err == nil && v >= 0 {
			userID = v
		}
	}
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
	if _, err := model.GetUserByID(req.ID); err != nil {
		common.ApiError(w, http.StatusNotFound, "user not found")
		return
	}
	fields := map[string]any{}
	if req.Quota != nil {
		if *req.Quota < 0 {
			common.ApiError(w, http.StatusBadRequest, "quota must be >= 0")
			return
		}
		if req.ExpectedQuota == nil {
			common.ApiError(w, http.StatusBadRequest, "expected_quota is required when setting a balance")
			return
		}
		fields["quota"] = *req.Quota
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
	if len(fields) == 0 {
		common.ApiError(w, http.StatusBadRequest, "nothing to update")
		return
	}
	if err := model.UpdateUserFieldsWithQuota(req.ID, fields, req.ExpectedQuota); err != nil {
		if err == model.ErrQuotaConflict {
			common.ApiError(w, http.StatusConflict, "balance changed; reload and confirm again")
			return
		}
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.ApiSuccess(w, nil)
}

func publicUser(u *model.User) map[string]any {
	return map[string]any{
		"id":         u.ID,
		"username":   u.Username,
		"email":      u.Email,
		"role":       u.Role,
		"status":     u.Status,
		"quota":      u.Quota,
		"used_quota": u.UsedQuota,
		"group":      u.Group,
		"aff_code":   u.AffCode,
		"inviter_id": u.InviterID,
		"created_at": u.CreatedAt,
	}
}

// --- Self-service profile updates ---

type updateSelfRequest struct {
	CurrentPassword *string `json:"current_password"`
	NewPassword     *string `json:"new_password"`
	Email           *string `json:"email"`
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
		if len(pw) < 6 || len(pw) > 64 {
			common.ApiError(w, http.StatusBadRequest, "password must be 6-64 characters")
			return
		}
		hashed, err := common.HashPassword(pw)
		if err != nil {
			common.ApiError(w, http.StatusInternalServerError, err.Error())
			return
		}
		fields["password"] = hashed
	}
	if req.Email != nil {
		email := strings.TrimSpace(*req.Email)
		if !validEmail(email) {
			common.ApiError(w, http.StatusBadRequest, "invalid email")
			return
		}
		fields["email"] = email
	}
	if len(fields) == 0 {
		common.ApiError(w, http.StatusBadRequest, "nothing to update")
		return
	}
	if err := model.UpdateUserFields(user.ID, fields); err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Return the updated public user so the caller can refresh state.
	updated, err := model.GetUserByID(user.ID)
	if err != nil {
		common.ApiSuccess(w, nil)
		return
	}
	common.ApiSuccess(w, publicUser(updated))
}

package controller

import (
	"net/http"
	"strings"
	"time"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
)

func staffAccountJSON(user model.User) map[string]any {
	return map[string]any{"platform_admin": user.HasPlatformWorkspace(), "id": user.ID, "username": user.Username, "role": user.Role, "status": user.Status, "version": user.SecurityVersion, "must_change_password": user.MustChangePassword, "two_factor_bound": user.TOTPSecret != "", "created_at": user.CreatedAt}
}

func ListStaffAccounts(w http.ResponseWriter, r *http.Request) {
	users, err := model.ListStaffAccounts()
	if err != nil {
		common.ApiError(w, 500, "staff accounts unavailable")
		return
	}
	items := make([]map[string]any, 0, len(users))
	for _, user := range users {
		items = append(items, staffAccountJSON(user))
	}
	common.ApiSuccess(w, items)
}

func CreateStaffAccount(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Username string `json:"username"`
		Password string `json:"temporary_password"`
		Role     int    `json:"role"`
		Reason   string `json:"reason"`
	}
	if !decodeJSONBody(w, r, &input) {
		return
	}
	if !validUsername(input.Username) || len(input.Password) < 12 || len(input.Password) > 72 || !common.IsStaffRole(input.Role) || !validStaffReason(input.Reason) {
		common.ApiError(w, 400, "valid username, staff role, 12–72 byte temporary password and reason are required")
		return
	}
	hash, err := common.HashPassword(input.Password)
	if err != nil {
		common.ApiError(w, 500, "staff account unavailable")
		return
	}
	user := &model.User{Username: input.Username, Password: hash, Role: input.Role, Status: common.StatusEnabled, Group: common.DefaultGroup, AffCode: common.GenerateAffCode()}
	if err := model.CreateStaffAccount(user, requestUserID(r), input.Reason, r.Context()); err != nil {
		common.ApiError(w, 409, "staff account could not be created; username may already exist")
		return
	}
	common.ApiSuccess(w, staffAccountJSON(*user))
}

func validStaffReason(value string) bool {
	return strings.TrimSpace(value) != "" && len(value) <= 500 && !strings.ContainsAny(value, "\x00\r\n")
}

func UpdateStaffAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var input struct {
		Version  int64  `json:"expected_version"`
		Role     int    `json:"role"`
		Status   int    `json:"status"`
		Reason   string `json:"reason"`
		ResetOTP bool   `json:"reset_2fa"`
		Password string `json:"current_password"`
		Code     string `json:"code"`
	}
	if !decodeJSONBody(w, r, &input) {
		return
	}
	if !validStaffReason(input.Reason) {
		common.ApiError(w, 400, "a reason of 1 to 500 bytes is required")
		return
	}
	actor := requestUserID(r)
	if input.ResetOTP {
		if id == actor {
			common.ApiError(w, 403, "another Root must reset 2FA; a sole Root uses the deployment CLI")
			return
		}
		user, err := model.GetStaffByID(actor)
		if err != nil || !common.VerifyPassword(input.Password, user.Password) {
			common.ApiError(w, 403, "fresh password and 2FA verification required")
			return
		}
		if _, err := model.VerifyStaffOTP(actor, user.SecurityVersion, input.Code, "", time.Now()); err != nil {
			common.ApiError(w, 403, "fresh password and 2FA verification required")
			return
		}
	}
	if err := model.UpdateStaffAccount(id, input.Version, input.Role, input.Status, input.ResetOTP, actor, input.Reason, r.Context()); err != nil {
		common.ApiError(w, 409, err.Error())
		return
	}
	middleware.Sessions.RemoveAllExcept(id, "")
	common.ApiSuccess(w, nil)
}

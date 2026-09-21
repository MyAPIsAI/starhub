package controller

import (
	"net/http"
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
	common.ApiSuccess(w, nil)
}

func GetSelf(w http.ResponseWriter, r *http.Request) {
	user, err := model.GetUserByID(requestUserID(r))
	if err != nil {
		common.ApiError(w, http.StatusNotFound, "user not found")
		return
	}
	common.ApiSuccess(w, publicUser(user))
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
	ID       int64   `json:"id"`
	Quota    *int64  `json:"quota"`
	Status   *int    `json:"status"`
	Group    *string `json:"group"`
	Password *string `json:"password"`
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
		fields["quota"] = *req.Quota
	}
	if req.Status != nil {
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
	if err := model.UpdateUserFields(req.ID, fields); err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.ApiSuccess(w, nil)
}

func publicUser(u *model.User) map[string]any {
	return map[string]any{
		"id":         u.ID,
		"username":   u.Username,
		"role":       u.Role,
		"status":     u.Status,
		"quota":      u.Quota,
		"used_quota": u.UsedQuota,
		"group":      u.Group,
		"created_at": u.CreatedAt,
	}
}

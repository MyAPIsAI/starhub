package controller

import (
	"net/http"

	"starhub/common"
	"starhub/model"
)

// GetProfileState exposes only account profile metadata after checking the
// caller's expected owner. Mutation support is advertised explicitly.
func GetProfileState(w http.ResponseWriter, r *http.Request) {
	owner, ok := expectedAccountOwner(w, r)
	if !ok {
		return
	}
	user, err := model.GetUserByID(owner)
	if err != nil {
		common.ApiError(w, http.StatusNotFound, "user not found")
		return
	}
	common.ApiSuccess(w, map[string]any{
		"schema":          "profile_state_v1",
		"guarded_updates": true,
		"user": map[string]any{
			"id": user.ID, "username": user.Username, "email": user.Email,
			"role": user.Role, "status": user.Status, "created_at": user.CreatedAt,
		},
	})
}

func GetUserSecurityState(w http.ResponseWriter, r *http.Request) {
	owner, ok := expectedAccountOwner(w, r)
	if !ok {
		return
	}
	events, err := model.ListUserSecurityEvents(owner)
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, "account security history is unavailable")
		return
	}
	common.ApiSuccess(w, map[string]any{"schema": "security_state_v1", "history_scope": "guarded_profile_changes_v1", "user_id": owner, "events": events})
}

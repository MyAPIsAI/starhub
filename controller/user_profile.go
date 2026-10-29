package controller

import (
	"net/http"

	"starhub/common"
	"starhub/model"
)

// GetProfileState exposes only account profile metadata after checking the
// caller's expected owner. It does not grant any profile mutation capability.
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
		"schema": "profile_state_v1",
		"user": map[string]any{
			"id": user.ID, "username": user.Username, "email": user.Email,
			"role": user.Role, "status": user.Status, "created_at": user.CreatedAt,
		},
	})
}

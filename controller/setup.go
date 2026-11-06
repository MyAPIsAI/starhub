package controller

import (
	"net/http"
	"os"
	"strings"
	"sync"

	"starhub/common"
	"starhub/model"
)

var (
	setupCompletedOnce sync.Once
	setupCompleted     bool
)

type setupRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Token    string `json:"token"`
}

// GetSetupHandler handles GET /api/setup.
// Returns whether setup is needed and whether a token is required.
func GetSetupHandler(w http.ResponseWriter, r *http.Request) {
	checkSetupCompleted()
	tokenRequired := os.Getenv("STARHUB_SETUP_TOKEN") != ""
	common.ApiSuccess(w, map[string]any{
		"setup_needed":   !setupCompleted,
		"token_required": tokenRequired,
	})
}

// PostSetupHandler handles POST /api/setup.
// Creates the first root user if setup has not been completed.
// When STARHUB_SETUP_TOKEN is set, the request must include the matching token.
func PostSetupHandler(w http.ResponseWriter, r *http.Request) {
	checkSetupCompleted()
	if setupCompleted {
		common.ApiError(w, http.StatusConflict, "setup has already been completed")
		return
	}
	var req setupRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	// Verify setup token if configured.
	expectedToken := os.Getenv("STARHUB_SETUP_TOKEN")
	if expectedToken != "" {
		if req.Token != expectedToken {
			common.ApiError(w, http.StatusForbidden, "invalid setup token")
			return
		}
	}
	// Validate username and password.
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
	// Check if a root user already exists.
	users, err := model.ListUsers()
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, u := range users {
		if u.Role == common.RoleRoot {
			setupCompleted = true
			common.ApiError(w, http.StatusConflict, "a root user already exists")
			return
		}
	}
	hashed, err := common.HashPassword(req.Password)
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	root := &model.User{
		Username: req.Username,
		Password: hashed,
		Role:     common.RoleRoot,
		Status:   common.StatusEnabled,
		Quota:    100_000_000,
		Group:    common.DefaultGroup,
		AffCode:  common.GenerateAffCode(),
	}
	if err := model.CreateUser(root); err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	setupCompleted = true
	common.ApiSuccess(w, map[string]any{
		"setup_completed": true,
		"username":        root.Username,
	})
}

func checkSetupCompleted() {
	setupCompletedOnce.Do(func() {
		users, err := model.ListUsers()
		if err != nil {
			return
		}
		for _, u := range users {
			if u.Role == common.RoleRoot {
				setupCompleted = true
				return
			}
		}
	})
}

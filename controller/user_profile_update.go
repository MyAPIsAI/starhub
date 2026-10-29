package controller

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/mail"
	"strings"

	"starhub/common"
	"starhub/model"
)

// UpdateProfileState accepts one owner-bound change at a time. It does not
// create sessions, alter other devices or recover credential material.
func UpdateProfileState(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ExpectedUserID  int64   `json:"expected_user_id"`
		ExpectedEmail   *string `json:"expected_email"`
		CurrentPassword *string `json:"current_password"`
		NewPassword     *string `json:"new_password"`
		Email           *string `json:"email"`
	}
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF || input.ExpectedUserID <= 0 || input.ExpectedEmail == nil || (input.NewPassword == nil) == (input.Email == nil) {
		common.ApiError(w, http.StatusBadRequest, "invalid account profile update")
		return
	}
	if input.ExpectedUserID != requestUserID(r) {
		common.ApiError(w, http.StatusConflict, "session identity changed; reload the account profile")
		return
	}
	owner, err := model.GetUserByID(input.ExpectedUserID)
	if err != nil || owner.Status != common.StatusEnabled {
		common.ApiError(w, http.StatusUnauthorized, "user unavailable")
		return
	}
	if owner.Email != *input.ExpectedEmail {
		common.ApiError(w, http.StatusConflict, "account profile changed; reload before updating")
		return
	}
	field, value := "email", ""
	if input.NewPassword != nil {
		if input.CurrentPassword == nil || len(*input.CurrentPassword) == 0 || len(*input.CurrentPassword) > 72 || len(*input.NewPassword) < 8 || len(*input.NewPassword) > 64 {
			common.ApiError(w, http.StatusBadRequest, "password must be 8-64 UTF-8 bytes and include the current password")
			return
		}
		if !common.VerifyPassword(*input.CurrentPassword, owner.Password) {
			common.ApiError(w, http.StatusForbidden, "current password is incorrect")
			return
		}
		field = "password"
		value, err = common.HashPassword(*input.NewPassword)
		if err != nil {
			common.ApiError(w, http.StatusInternalServerError, "password update could not be prepared")
			return
		}
	} else {
		if input.CurrentPassword != nil || *input.Email != strings.TrimSpace(*input.Email) || !validEmail(*input.Email) {
			common.ApiError(w, http.StatusBadRequest, "invalid profile email")
			return
		}
		value = *input.Email
		if value != "" {
			address, err := mail.ParseAddress(value)
			if err != nil || address.Address != value {
				common.ApiError(w, http.StatusBadRequest, "invalid profile email")
				return
			}
		}
	}
	if err := model.UpdateOwnedProfile(owner, field, value); err != nil {
		if errors.Is(err, model.ErrUserProfileConflict) {
			common.ApiError(w, http.StatusConflict, "account profile changed; reload before updating")
			return
		}
		common.ApiError(w, http.StatusInternalServerError, "account profile update could not be confirmed")
		return
	}
	common.ApiSuccess(w, map[string]any{"user_id": owner.ID, "updated_field": field})
}

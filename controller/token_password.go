package controller

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"starhub/common"
	"starhub/model"
)

// VerifyTokenPassword checks the current owner's password without creating a
// login session, rotating cookies or returning any API credential material.
func VerifyTokenPassword(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	var input struct {
		ExpectedUserID int64  `json:"expected_user_id"`
		Password       string `json:"password"`
	}
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF || input.ExpectedUserID <= 0 || len(input.Password) == 0 || len(input.Password) > 72 {
		common.ApiError(w, http.StatusBadRequest, "invalid password verification request")
		return
	}
	userID := requestUserID(r)
	if userID <= 0 || input.ExpectedUserID != userID {
		common.ApiError(w, http.StatusConflict, "session identity changed; reopen the verification")
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	token, err := model.GetTokenByID(id)
	if err != nil {
		if errors.Is(err, model.ErrTokenNotFound) {
			common.ApiError(w, http.StatusNotFound, "token not found")
		} else {
			common.ApiError(w, http.StatusInternalServerError, "token ownership could not be verified")
		}
		return
	}
	// Management roles do not bypass ownership for a personal unlock.
	if token.UserID != userID {
		common.ApiError(w, http.StatusNotFound, "token not found")
		return
	}
	user, err := model.GetUserByID(userID)
	if err != nil || user.Status != common.StatusEnabled || !common.VerifyPassword(input.Password, user.Password) {
		common.ApiError(w, http.StatusForbidden, "password verification failed")
		return
	}
	currentUser, userErr := model.GetUserByID(userID)
	currentToken, tokenErr := model.GetTokenByID(id)
	if userErr != nil || tokenErr != nil || currentUser.Status != common.StatusEnabled || currentUser.Password != user.Password || currentToken.UserID != userID || currentToken.Key != token.Key {
		common.ApiError(w, http.StatusConflict, "account or token changed; reopen the verification")
		return
	}
	common.ApiSuccess(w, map[string]any{"verified": true, "user_id": userID, "token_id": id})
}

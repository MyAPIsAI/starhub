package controller

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
)

// VerifyTokenPassword unlocks only the current owner's key after reauthentication.
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
	grant, expires, err := model.NewTokenRevealGrant(currentUser, currentToken, middleware.SessionToken(r))
	if err != nil {
		common.ApiError(w, 500, "key viewing authorization unavailable")
		return
	}
	writeTokenReveal(w, r, currentUser, currentToken, grant, expires)
}

func RevealToken(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	var input struct {
		ExpectedUserID int64  `json:"expected_user_id"`
		Grant          string `json:"grant"`
	}
	defer r.Body.Close()
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	d.DisallowUnknownFields()
	if d.Decode(&input) != nil || d.Decode(new(any)) != io.EOF || input.ExpectedUserID <= 0 || len(input.Grant) != 64 {
		common.ApiError(w, 400, "invalid key viewing authorization")
		return
	}
	if input.ExpectedUserID != requestUserID(r) {
		common.ApiError(w, 409, "current account changed")
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	token, err := model.GetOwnedTokenByID(input.ExpectedUserID, id)
	if err != nil {
		common.ApiError(w, 404, "token not found")
		return
	}
	user, err := model.GetUserByID(input.ExpectedUserID)
	if err != nil || user.Status != common.StatusEnabled {
		common.ApiError(w, 403, "account unavailable")
		return
	}
	expires, err := model.CheckTokenRevealGrant(input.Grant, middleware.SessionToken(r), user, token)
	if err != nil {
		common.ApiError(w, 403, err.Error())
		return
	}
	writeTokenReveal(w, r, user, token, input.Grant, expires)
}

func writeTokenReveal(w http.ResponseWriter, r *http.Request, user *model.User, token *model.Token, grant string, expires int64) {
	audit := &model.AuditLog{UserID: user.ID, Username: user.Username,
		Method: r.Method, Path: r.URL.Path, Status: http.StatusOK, IP: common.ClientIP(r)}
	if err := model.InsertAuditLog(audit); err != nil {
		common.ApiError(w, http.StatusInternalServerError, "key viewing could not be audited")
		return
	}
	common.ApiSuccess(w, map[string]any{"verified": true, "user_id": user.ID,
		"token_id": token.ID, "key": token.Key, "expires_at": expires, "grant": grant})
}

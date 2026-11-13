package controller

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
)

// VerifyTokenPassword checks the current owner's password without creating a
// login session, rotating cookies or returning any API credential material.
func VerifyTokenPassword(w http.ResponseWriter, r *http.Request) {
	verifyTokenPassword(w, r, false)
}

func UnlockToken(w http.ResponseWriter, r *http.Request) {
	verifyTokenPassword(w, r, true)
}

func verifyTokenPassword(w http.ResponseWriter, r *http.Request, unlock bool) {
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
	if userErr != nil || tokenErr != nil || currentUser.Status != common.StatusEnabled || currentUser.Password != user.Password || currentToken.UserID != userID || currentToken.KeyHash != token.KeyHash {
		common.ApiError(w, http.StatusConflict, "account or token changed; reopen the verification")
		return
	}
	if unlock && (!tokenReadable(currentToken, time.Now()) || !middleware.Sessions.UnlockToken(middleware.SessionToken(r), userID, id, currentUser.Password, tokenSecretFingerprint(currentToken), time.Now())) {
		common.ApiError(w, http.StatusForbidden, "active session and key required")
		return
	}
	common.ApiSuccess(w, map[string]any{"verified": true, "user_id": userID, "token_id": id})
}

func tokenSecretFingerprint(token *model.Token) string {
	return common.HashKey(token.KeyHash + "|" + token.Key)
}
func tokenReadable(token *model.Token, now time.Time) bool {
	return token.Status == common.StatusEnabled && (token.ExpiredTime <= 0 || now.Unix() < token.ExpiredTime)
}

// RevealToken never caches plaintext in the session or substitutes ciphertext
// when decryption fails. Every read is fenced by the current credentials.
func RevealToken(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	var input struct {
		ExpectedUserID int64 `json:"expected_user_id"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	d.DisallowUnknownFields()
	if d.Decode(&input) != nil || d.Decode(new(any)) != io.EOF || input.ExpectedUserID <= 0 || input.ExpectedUserID != requestUserID(r) {
		common.ApiError(w, 409, "current account changed")
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	token, err := model.GetTokenByID(id)
	if err != nil || token.UserID != input.ExpectedUserID {
		common.ApiError(w, 404, "token not found")
		return
	}
	user, err := model.GetUserByID(input.ExpectedUserID)
	if err != nil || user.Status != common.StatusEnabled || !tokenReadable(token, time.Now()) {
		common.ApiError(w, 403, "account or key unavailable")
		return
	}
	expires, ok := middleware.Sessions.TokenUnlockExpiry(middleware.SessionToken(r), user.ID, id, user.Password, tokenSecretFingerprint(token), time.Now())
	if !ok {
		common.ApiError(w, 403, "password verification required")
		return
	}
	secret, err := common.Decrypt(token.Key)
	if err != nil || secret == "" {
		common.ApiError(w, 500, "key could not be decrypted")
		return
	}
	if err := model.InsertAuditLog(&model.AuditLog{UserID: user.ID, Username: user.Username, Method: r.Method, Path: r.URL.Path, Status: 200, IP: clientIP(r), RequestID: middleware.CtxRequestIDValue(r.Context())}); err != nil {
		common.ApiError(w, 500, "key access could not be recorded")
		return
	}
	common.ApiSuccess(w, map[string]any{"user_id": user.ID, "token_id": id, "key": secret, "expires_at": expires.UnixMilli()})
}

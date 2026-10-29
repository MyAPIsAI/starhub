package controller

import (
	"encoding/json"
	"io"
	"net/http"

	"starhub/common"
	"starhub/middleware"
)

// LogoutCurrent revokes only the request's session and never changes browser
// cookies, so a delayed response cannot clear a newer login's cookie.
func LogoutCurrent(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	var input struct {
		Schema         string `json:"schema"`
		ExpectedUserID int64  `json:"expected_user_id"`
	}
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF || input.Schema != "logout_current_v1" || input.ExpectedUserID <= 0 {
		common.ApiError(w, http.StatusBadRequest, "invalid current-session logout")
		return
	}
	if input.ExpectedUserID != requestUserID(r) {
		common.ApiError(w, http.StatusConflict, "session identity changed; confirm the current account")
		return
	}
	middleware.Sessions.Remove(middleware.SessionToken(r))
	common.ApiSuccess(w, map[string]any{"schema": "logout_receipt_v1", "user_id": input.ExpectedUserID, "signed_out": true})
}

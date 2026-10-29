package controller

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"starhub/common"
	"starhub/middleware"
)

// GetSessionState exposes an explicit guarded-management contract. Legacy
// session-list clients keep their array response on the original endpoint.
func GetSessionState(w http.ResponseWriter, r *http.Request) {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(query) != 1 || len(query["expected_user_id"]) != 1 {
		common.ApiError(w, http.StatusBadRequest, "a single expected_user_id is required")
		return
	}
	value := query.Get("expected_user_id")
	expected, err := strconv.ParseInt(value, 10, 64)
	if err != nil || expected <= 0 || strconv.FormatInt(expected, 10) != value {
		common.ApiError(w, http.StatusBadRequest, "invalid expected_user_id")
		return
	}
	if expected != requestUserID(r) {
		common.ApiError(w, http.StatusConflict, "session identity changed; refresh the account")
		return
	}
	sessions := middleware.Sessions.ListByUser(expected, middleware.SessionToken(r))
	if sessions == nil {
		sessions = []middleware.SessionInfo{}
	}
	common.ApiSuccess(w, map[string]any{"schema": "session_state_v1", "guarded_management": true, "user_id": expected, "sessions": sessions})
}

func sessionTerminationOwner(w http.ResponseWriter, r *http.Request) (int64, bool) {
	owner := requestUserID(r)
	if owner <= 0 {
		common.ApiError(w, http.StatusUnauthorized, "login required")
		return 0, false
	}
	defer r.Body.Close()
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4<<10))
	if err != nil {
		common.ApiError(w, http.StatusBadRequest, "invalid session termination request")
		return 0, false
	}
	if len(bytes.TrimSpace(body)) == 0 {
		// The historical endpoint accepted an empty body. Modern clients first
		// require the guarded read contract and always send the expected owner.
		return owner, true
	}
	var input struct {
		ExpectedUserID int64 `json:"expected_user_id"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF || input.ExpectedUserID <= 0 {
		common.ApiError(w, http.StatusBadRequest, "invalid session termination request")
		return 0, false
	}
	if input.ExpectedUserID != owner {
		common.ApiError(w, http.StatusConflict, "session identity changed; refresh the account")
		return 0, false
	}
	return owner, true
}

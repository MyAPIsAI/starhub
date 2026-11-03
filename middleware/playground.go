package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"starhub/model"
)

// A browser playground call must bind the session, selected key and reveal grant.
func authorizePlayground(w http.ResponseWriter, r *http.Request, token *model.Token) bool {
	if r.Header.Get("X-MyAPIs-Source") != "playground" {
		return true
	}
	owner, ok := Sessions.Get(SessionToken(r))
	id, err := strconv.ParseInt(r.Header.Get("X-MyAPIs-Key-ID"), 10, 64)
	if !ok || err != nil || owner != token.UserID || id != token.ID {
		writeRelayRequestError(w, r, 404, "invalid_request_error", "key_not_found", "key not found")
		return false
	}
	user, err := model.GetUserByID(owner)
	if err != nil {
		writeRelayRequestError(w, r, 404, "invalid_request_error", "key_not_found", "key not found")
		return false
	}
	if _, err := model.CheckTokenRevealGrant(r.Header.Get("X-MyAPIs-Key-Grant"), SessionToken(r), user, token); err != nil {
		writeRelayRequestError(w, r, 403, "authentication_error", "key_verification_required", "verify your password before using this key")
		return false
	}
	if r.Method != http.MethodPost || (r.URL.Path != "/v1/chat/completions" && r.URL.Path != "/v1/messages" && !strings.HasPrefix(r.URL.Path, "/v1beta/models/")) {
		writeRelayRequestError(w, r, 400, "invalid_request_error", "playground_text_only", "the playground supports text chat only")
		return false
	}
	return true
}

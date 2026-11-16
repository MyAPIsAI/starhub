package controller

import (
	"net/http"
	"strconv"
	"time"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
)

// Bind grants to configuration revisions so disabling and re-enabling a key
// cannot revive its old authorization. Usage settlement does not change this revision.
func playgroundTokenFingerprint(token *model.Token) string {
	return common.HashKey(strconv.FormatInt(token.Revision, 10) + "|" + token.KeyHash + "|" + token.Key)
}

func playgroundIdentity(w http.ResponseWriter, r *http.Request) (*model.User, *model.Token, bool) {
	w.Header().Set("Cache-Control", "private, no-store")
	id, ok := pathID(w, r, "id")
	if !ok {
		return nil, nil, false
	}
	user, err := model.GetUserByID(requestUserID(r))
	if err != nil || user.Status != common.StatusEnabled {
		common.ApiError(w, http.StatusUnauthorized, "user unavailable")
		return nil, nil, false
	}
	token, err := model.GetTokenByID(id)
	if err != nil || token.UserID != user.ID {
		common.ApiError(w, http.StatusNotFound, "token not found")
		return nil, nil, false
	}
	if !tokenReadable(token, time.Now()) {
		common.ApiError(w, http.StatusForbidden, "account or key unavailable")
		return nil, nil, false
	}
	return user, token, true
}

func PlaygroundTokenStatus(w http.ResponseWriter, r *http.Request) {
	user, token, ok := playgroundIdentity(w, r)
	if !ok {
		return
	}
	unlocked := middleware.Sessions.PlaygroundTokenUnlocked(middleware.SessionToken(r), user, token.ID, playgroundTokenFingerprint(token), time.Now())
	common.ApiSuccess(w, map[string]any{"user_id": user.ID, "token_id": token.ID, "unlocked": unlocked})
}

// PlaygroundRelay admits a session-owned grant before entering the complete
// API-key relay chain. Client credentials cannot select a different key.
func PlaygroundRelay(next http.Handler, relayPath string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, token, ok := playgroundIdentity(w, r)
		if !ok {
			return
		}
		if !middleware.Sessions.PlaygroundTokenUnlocked(middleware.SessionToken(r), user, token.ID, playgroundTokenFingerprint(token), time.Now()) {
			common.ApiError(w, http.StatusForbidden, "playground_unlock_required")
			return
		}
		secret, err := common.Decrypt(token.Key)
		if err != nil || secret == "" {
			common.ApiError(w, http.StatusInternalServerError, "key could not be decrypted")
			return
		}
		forward := r.Clone(r.Context())
		forward.Header.Del("Authorization")
		forward.Header.Del("X-Api-Key")
		forward.Header.Del("X-Goog-Api-Key")
		forward.Header.Set("Authorization", "Bearer "+secret)
		forward.URL.Path = relayPath
		if relayPath == "/v1beta/models/" {
			forward.URL.Path += r.PathValue("method")
		}
		forward.URL.RawPath = ""
		forward.RequestURI = forward.URL.RequestURI()
		next.ServeHTTP(w, forward)
	})
}

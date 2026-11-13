package controller

import (
	"net/http"
	"sync"
	"time"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
)

const loginChallengeCookie = "starhub_login_challenge"

type loginChallenge struct {
	UserID                       int64
	Password, Secret, Device, IP string
	Role                         int
	DeactivationVersion          int64
	Expires                      time.Time
}

var loginChallenges = struct {
	sync.Mutex
	items map[string]loginChallenge
}{items: make(map[string]loginChallenge)}

// beginTwoFactor issues a bounded, single-use password proof in an HttpOnly
// cookie. It also fails closed when the second-factor store is unavailable.
func beginTwoFactor(w http.ResponseWriter, r *http.Request, user *model.User) bool {
	factor, err := model.GetTwoFA(user.ID)
	if err == model.Err2FANotEnabled || (err == nil && !factor.IsEnabled) {
		return false
	}
	if err != nil {
		common.ApiError(w, http.StatusServiceUnavailable, "authentication unavailable")
		return true
	}
	now := time.Now()
	loginChallenges.Lock()
	for key, proof := range loginChallenges.items {
		if !now.Before(proof.Expires) {
			delete(loginChallenges.items, key)
		}
	}
	if old, err := r.Cookie(loginChallengeCookie); err == nil {
		delete(loginChallenges.items, old.Value)
	}
	if len(loginChallenges.items) >= 4096 {
		loginChallenges.Unlock()
		common.ApiError(w, http.StatusServiceUnavailable, "authentication capacity exceeded; retry later")
		return true
	}
	key := common.GenerateSessionKey()
	loginChallenges.items[key] = loginChallenge{UserID: user.ID, Password: user.Password,
		Secret: factor.Secret, Role: user.Role, DeactivationVersion: user.DeactivationVersion,
		Device: r.UserAgent(), IP: clientIP(r), Expires: now.Add(5 * time.Minute)}
	loginChallenges.Unlock()
	http.SetCookie(w, &http.Cookie{Name: loginChallengeCookie, Value: key,
		Path: "/api/user/login", HttpOnly: true, Secure: middleware.SecureSessionCookie(r),
		SameSite: http.SameSiteStrictMode, MaxAge: 300})
	common.ApiSuccess(w, map[string]any{"2fa_required": true, "username": user.Username})
	return true
}

func consumeLoginChallenge(w http.ResponseWriter, r *http.Request) (loginChallenge, bool) {
	http.SetCookie(w, &http.Cookie{Name: loginChallengeCookie, Path: "/api/user/login",
		HttpOnly: true, Secure: middleware.SecureSessionCookie(r), SameSite: http.SameSiteStrictMode, MaxAge: -1})
	cookie, err := r.Cookie(loginChallengeCookie)
	if err != nil {
		return loginChallenge{}, false
	}
	loginChallenges.Lock()
	proof, ok := loginChallenges.items[cookie.Value]
	delete(loginChallenges.items, cookie.Value)
	loginChallenges.Unlock()
	return proof, ok && time.Now().Before(proof.Expires) && proof.Device == r.UserAgent() && proof.IP == clientIP(r)
}

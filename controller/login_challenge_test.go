package controller

import (
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"starhub/common"
	"starhub/model"
)

func TestTwoFactorLoginRequiresSingleUsePasswordProof(t *testing.T) {
	newTestDB(t)
	user := newUser(t, "factor-user", common.RoleUser, 0)
	key, err := common.GenerateTOTPSecret(user.Username)
	if err != nil {
		t.Fatal(err)
	}
	if err := model.DB.Create(&model.TwoFA{UserID: user.ID, Secret: key.Secret(), IsEnabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	code, err := totp.GenerateCode(key.Secret(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"username":%q,"code":%q}`, user.Username, code)
	withoutProof := call(Verify2FALogin, testRequest{method: "POST", target: "/api/user/login/2fa", body: body})
	expectStatus(t, withoutProof, http.StatusUnauthorized)

	for _, mode := range []string{"valid", "expired", "password_changed", "wrong_account", "wrong_device", "factor_changed", "disabled"} {
		t.Run(mode, func(t *testing.T) {
			login := call(Login, testRequest{method: "POST", target: "/api/user/login", body: `{"username":"factor-user","password":"secret"}`})
			expectStatus(t, login, http.StatusOK)
			var cookie *http.Cookie
			for _, c := range login.Result().Cookies() {
				if c.Name == "session" {
					t.Fatal("password step issued a session")
				}
				if c.Name == loginChallengeCookie {
					cookie = c
				}
			}
			if cookie == nil || !cookie.HttpOnly {
				t.Fatal("missing protected password proof")
			}
			headers := map[string]string{"Cookie": cookie.String()}
			requestBody := body
			loginChallenges.Lock()
			proof := loginChallenges.items[cookie.Value]
			switch mode {
			case "expired":
				proof.Expires = time.Now().Add(-time.Second)
			case "password_changed":
				proof.Password = "obsolete"
			case "factor_changed":
				proof.Secret = "obsolete"
			case "disabled":
				proof.DeactivationVersion++
			case "wrong_account":
				requestBody = fmt.Sprintf(`{"username":"another-user","code":%q}`, code)
			case "wrong_device":
				headers["User-Agent"] = "different browser"
			}
			loginChallenges.items[cookie.Value] = proof
			loginChallenges.Unlock()
			verified := call(Verify2FALogin, testRequest{method: "POST", target: "/api/user/login/2fa", body: requestBody, headers: headers})
			want := http.StatusUnauthorized
			if mode == "valid" {
				want = http.StatusOK
			}
			expectStatus(t, verified, want)
			if mode == "valid" {
				found := false
				for _, c := range verified.Result().Cookies() {
					found = found || (c.Name == "session" && c.Value != "")
				}
				if !found {
					t.Fatal("valid second factor did not issue a session")
				}
			}
			replayed := call(Verify2FALogin, testRequest{method: "POST", target: "/api/user/login/2fa", body: body, headers: headers})
			expectStatus(t, replayed, http.StatusUnauthorized)
		})
	}
}

func TestConcurrentTwoFactorSubmissionsConsumePasswordProofOnce(t *testing.T) {
	newTestDB(t)
	user := newUser(t, "concurrent-factor", common.RoleUser, 0)
	key, err := common.GenerateTOTPSecret(user.Username)
	if err != nil {
		t.Fatal(err)
	}
	if err := model.DB.Create(&model.TwoFA{UserID: user.ID, Secret: key.Secret(), IsEnabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	login := call(Login, testRequest{method: "POST", target: "/api/user/login", body: `{"username":"concurrent-factor","password":"secret"}`})
	expectStatus(t, login, http.StatusOK)
	var proof *http.Cookie
	for _, cookie := range login.Result().Cookies() {
		if cookie.Name == loginChallengeCookie {
			proof = cookie
		}
		if cookie.Name == "session" {
			t.Fatal("password step issued a session")
		}
	}
	if proof == nil {
		t.Fatal("missing password proof")
	}
	code, err := totp.GenerateCode(key.Secret(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"username":%q,"code":%q}`, user.Username, code)
	type result struct {
		status  int
		session bool
	}
	results := make(chan result, 16)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			response := call(Verify2FALogin, testRequest{method: "POST", target: "/api/user/login/2fa", body: body, headers: map[string]string{"Cookie": proof.String()}})
			hasSession := false
			for _, cookie := range response.Result().Cookies() {
				hasSession = hasSession || cookie.Name == "session" && cookie.Value != ""
			}
			results <- result{response.Code, hasSession}
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	success := 0
	for r := range results {
		if r.status == http.StatusOK && r.session {
			success++
		} else if r.status != http.StatusUnauthorized || r.session {
			t.Fatalf("unexpected result %+v", r)
		}
	}
	if success != 1 {
		t.Fatalf("issued %d sessions for one password proof", success)
	}
	replay := call(Verify2FALogin, testRequest{method: "POST", target: "/api/user/login/2fa", body: body, headers: map[string]string{"Cookie": proof.String()}})
	expectStatus(t, replay, http.StatusUnauthorized)
}

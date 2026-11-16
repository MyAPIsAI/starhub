package router

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
	"starhub/setting"
)

type playgroundFixture struct {
	handler http.Handler
	user    *model.User
	token   *model.Token
	session string
	hits    atomic.Int32
}

func newPlaygroundFixture(t *testing.T) *playgroundFixture {
	t.Helper()
	setting.ResetAllToDefaults()
	middleware.ResetRateLimits()
	middleware.ResetTokenRateLimits()
	middleware.ResetModelRateLimits()
	if err := model.InitDB(filepath.Join(t.TempDir(), "playground.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { model.CloseDB(); setting.ResetAllToDefaults() })
	if err := model.LoadOptionsToSettings(); err != nil {
		t.Fatal(err)
	}
	password, _ := common.HashPassword("PlaygroundFixture92")
	u := &model.User{Username: "playground-owner", Password: password, Status: 1, Role: 1, Quota: 1000000, Group: "default", AffCode: common.GenerateAffCode()}
	if err := model.CreateUser(u); err != nil {
		t.Fatal(err)
	}
	token := &model.Token{UserID: u.ID, Name: "playground-key", Status: 1, ExpiredTime: -1, RemainQuota: 1000000, Group: "default", ModelLimits: "playground-model"}
	if err := model.InsertToken(token); err != nil {
		t.Fatal(err)
	}
	f := &playgroundFixture{user: u, token: token}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits.Add(1)
		if strings.HasPrefix(r.URL.Path, "/v1beta/") {
			if r.Header.Get("X-Goog-Api-Key") != "fixture-upstream" {
				t.Error("unexpected Gemini credential")
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"Playground OK"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":7,"candidatesTokenCount":3,"totalTokenCount":10},"modelVersion":"playground-model"}`)
			return
		}
		if r.Header.Get("Authorization") != "Bearer fixture-upstream" {
			t.Error("unexpected upstream credential")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"chatcmpl-fixture","object":"chat.completion","model":"playground-model","choices":[{"index":0,"message":{"role":"assistant","content":"Playground OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":3,"total_tokens":10}}`)
	}))
	t.Cleanup(upstream.Close)
	if err := model.InsertChannel(&model.Channel{Name: "fixture", Type: 1, Key: "fixture-upstream", BaseURL: upstream.URL, Models: "playground-model", Group: "default", Status: 1, Weight: 1}); err != nil {
		t.Fatal(err)
	}
	if err := model.InsertChannel(&model.Channel{Name: "gemini-fixture", Type: 25, Key: "fixture-upstream", BaseURL: upstream.URL, Models: "playground-model", Group: "default", Status: 1, Priority: -1, Weight: 1}); err != nil {
		t.Fatal(err)
	}
	f.session = middleware.Sessions.CreateConfirmed(u, time.Hour, "127.0.0.1", "fixture")
	t.Cleanup(func() { middleware.Sessions.Remove(f.session) })
	f.handler = New()
	return f
}

func (f *playgroundFixture) call(method, path, body, session string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://example.test"+path, strings.NewReader(body))
	r.RemoteAddr = "127.0.0.1:12345"
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "http://example.test")
	if session != "" {
		r.AddCookie(&http.Cookie{Name: "session", Value: session})
	}
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	return w
}

func (f *playgroundFixture) path(action string) string {
	return fmt.Sprintf("/api/playground/%d/%s", f.token.ID, action)
}

func (f *playgroundFixture) unlock(t *testing.T) {
	t.Helper()
	w := f.call("POST", f.path("unlock"), fmt.Sprintf(`{"expected_user_id":%d,"password":"PlaygroundFixture92"}`, f.user.ID), f.session)
	if w.Code != 200 {
		t.Fatalf("unlock: %d %s", w.Code, w.Body)
	}
	if strings.Contains(w.Body.String(), f.token.PlaintextKey()) || strings.Contains(w.Body.String(), f.token.Key) {
		t.Fatal("unlock disclosed credentials")
	}
}

const playgroundChatBody = `{"model":"playground-model","messages":[{"role":"user","content":"Hi"}],"max_tokens":32}`

func TestPlaygroundUsesExistingRelayAuthorizationAndBilling(t *testing.T) {
	f := newPlaygroundFixture(t)
	if w := f.call("POST", f.path("chat/completions"), playgroundChatBody, ""); w.Code != 401 {
		t.Fatalf("anonymous: %d", w.Code)
	}
	if w := f.call("POST", f.path("chat/completions"), playgroundChatBody, f.session); w.Code != 403 {
		t.Fatalf("locked: %d", w.Code)
	}
	wrong := fmt.Sprintf(`{"expected_user_id":%d,"password":"wrong"}`, f.user.ID)
	if w := f.call("POST", f.path("unlock"), wrong, f.session); w.Code != 403 {
		t.Fatalf("wrong password: %d", w.Code)
	}
	f.unlock(t)
	for _, tc := range []struct{ action, body string }{
		{"chat/completions", playgroundChatBody},
		{"messages", playgroundChatBody},
		{"gemini/playground-model:generateContent", `{"contents":[{"role":"user","parts":[{"text":"Hi"}]}],"generationConfig":{"maxOutputTokens":32}}`},
	} {
		w := f.call("POST", f.path(tc.action), tc.body, f.session)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "Playground OK") {
			t.Fatalf("%s: %d %s", tc.action, w.Code, w.Body)
		}
	}
	if f.hits.Load() != 3 {
		t.Fatalf("upstream calls: %d", f.hits.Load())
	}
	token, _ := model.GetTokenByID(f.token.ID)
	if token.UsedQuota <= 0 {
		t.Fatal("relay did not settle usage")
	}
	var logs []model.Log
	model.DB.Find(&logs)
	if len(logs) != 3 {
		t.Fatalf("usage logs: %d", len(logs))
	}
	body := strings.Replace(playgroundChatBody, "playground-model", "unauthorized-model", 1)
	if w := f.call("POST", f.path("chat/completions"), body, f.session); w.Code != 403 {
		t.Fatalf("model allowlist: %d %s", w.Code, w.Body)
	}
	model.UpdateTokenFields(f.token.ID, map[string]any{"allow_ips": "192.0.2.1"})
	f.unlock(t)
	if w := f.call("POST", f.path("chat/completions"), playgroundChatBody, f.session); w.Code != 403 || !strings.Contains(w.Body.String(), "ip_not_allowed") {
		t.Fatalf("IP allowlist: %d %s", w.Code, w.Body)
	}
	model.UpdateTokenFields(f.token.ID, map[string]any{"allow_ips": "", "remain_quota": int64(0), "remain_quota_remainder_nano": int64(0)})
	f.unlock(t)
	if w := f.call("POST", f.path("chat/completions"), playgroundChatBody, f.session); w.Code != 403 || !strings.Contains(w.Body.String(), "token_quota_exhausted") {
		t.Fatalf("quota: %d %s", w.Code, w.Body)
	}
	if f.hits.Load() != 3 {
		t.Fatal("rejected calls reached upstream")
	}
}

func TestPlaygroundGrantDoesNotAuthorizeSecretReadsOrOtherSessions(t *testing.T) {
	f := newPlaygroundFixture(t)
	f.unlock(t)
	ownerBody := fmt.Sprintf(`{"expected_user_id":%d}`, f.user.ID)
	if w := f.call("POST", fmt.Sprintf("/api/token/%d/reveal", f.token.ID), ownerBody, f.session); w.Code != 403 {
		t.Fatal("playground unlock authorized a secret read")
	}
	otherSession := middleware.Sessions.CreateConfirmed(f.user, time.Hour, "", "other")
	defer middleware.Sessions.Remove(otherSession)
	if w := f.call("POST", f.path("chat/completions"), playgroundChatBody, otherSession); w.Code != 403 {
		t.Fatal("grant crossed sessions")
	}
	otherToken := &model.Token{UserID: f.user.ID, Name: "other", Status: 1, RemainQuota: 1000, ExpiredTime: -1}
	if err := model.InsertToken(otherToken); err != nil {
		t.Fatal(err)
	}
	if w := f.call("POST", fmt.Sprintf("/api/playground/%d/chat/completions", otherToken.ID), playgroundChatBody, f.session); w.Code != 403 {
		t.Fatal("grant crossed keys")
	}
	otherUser := &model.User{Username: "other-owner", Password: f.user.Password, Role: 100, Status: 1, Group: "default", AffCode: common.GenerateAffCode(), AdminPasswordChangedAt: time.Now().Unix()}
	if err := model.CreateUser(otherUser); err != nil {
		t.Fatal(err)
	}
	// Root ownership must be checked even when staff security is bypassed in development.
	t.Setenv("STARHUB_ENV", "development")
	rootSession := middleware.Sessions.CreateConfirmed(otherUser, time.Hour, "", "root")
	defer middleware.Sessions.Remove(rootSession)
	middleware.Sessions.SkipStaffTwoFA(rootSession, otherUser)
	if w := f.call("GET", f.path("status"), "", rootSession); w.Code != 404 {
		t.Fatalf("root read another owner's key: %d", w.Code)
	}
	if f.hits.Load() != 0 {
		t.Fatal("unverified request reached upstream")
	}
}

func TestPlaygroundRevokesGrantsWhenSecurityStateChanges(t *testing.T) {
	for _, change := range []string{"password", "reset", "disabled", "expired", "deleted", "logout", "account disabled", "generation"} {
		t.Run(change, func(t *testing.T) {
			f := newPlaygroundFixture(t)
			f.unlock(t)
			switch change {
			case "password":
				hash, _ := common.HashPassword("ChangedFixture93")
				model.DB.Model(f.user).Update("password", hash)
			case "reset":
				model.UpdateTokenFields(f.token.ID, map[string]any{"key_hash": "rotated"})
			case "disabled":
				model.UpdateTokenFields(f.token.ID, map[string]any{"status": 2})
				model.UpdateTokenFields(f.token.ID, map[string]any{"status": 1})
			case "expired":
				model.UpdateTokenFields(f.token.ID, map[string]any{"expired_time": time.Now().Add(-time.Hour).Unix()})
			case "deleted":
				model.DeleteToken(f.token.ID)
			case "logout":
				middleware.Sessions.Remove(f.session)
			case "account disabled":
				model.DB.Model(f.user).Update("status", 2)
			case "generation":
				model.DB.Model(f.user).Update("deactivation_version", 1)
			}
			w := f.call("POST", f.path("chat/completions"), playgroundChatBody, f.session)
			if w.Code < 400 || f.hits.Load() != 0 {
				t.Fatalf("%s retained access: %d %s", change, w.Code, w.Body)
			}
		})
	}
}

func TestPlaygroundSecretRevealGrantRemainsSeparate(t *testing.T) {
	f := newPlaygroundFixture(t)
	w := f.call("POST", fmt.Sprintf("/api/token/%d/unlock", f.token.ID), fmt.Sprintf(`{"expected_user_id":%d,"password":"PlaygroundFixture92"}`, f.user.ID), f.session)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	w = f.call("GET", f.path("status"), "", f.session)
	var result struct {
		Data struct {
			Unlocked bool `json:"unlocked"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &result)
	if w.Code != 200 || result.Data.Unlocked {
		t.Fatal("short secret grant authorized session-long playground use")
	}
}

func TestPlaygroundEnforcesBrowserAndOwnerBoundaries(t *testing.T) {
	f := newPlaygroundFixture(t)
	f.unlock(t)
	for _, tc := range []struct {
		name, origin, expectedOwner string
		status                      int
	}{
		{"cross origin", "https://other.example", "", 403},
		{"changed owner", "http://example.test", fmt.Sprint(f.user.ID + 1), 409},
		{"owned key", "http://example.test", fmt.Sprint(f.user.ID), 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "http://example.test"+f.path("chat/completions"), strings.NewReader(playgroundChatBody))
			r.RemoteAddr = "127.0.0.1:12345"
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Origin", tc.origin)
			if tc.expectedOwner != "" {
				r.Header.Set("X-StarHub-Expected-User-ID", tc.expectedOwner)
			}
			// Client-supplied credentials must never override the unlocked key.
			r.Header.Set("Authorization", "Bearer invalid-client-key")
			r.Header.Set("X-Api-Key", "invalid-client-key")
			r.Header.Set("X-Goog-Api-Key", "invalid-client-key")
			r.AddCookie(&http.Cookie{Name: "session", Value: f.session})
			w := httptest.NewRecorder()
			f.handler.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("status: %d %s", w.Code, w.Body)
			}
		})
	}
	if f.hits.Load() != 1 {
		t.Fatalf("upstream calls: %d", f.hits.Load())
	}
}

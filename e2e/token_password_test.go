package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
)

func TestTokenPasswordActualRouterNeverCreatesOrReplacesLoginSessions(t *testing.T) {
	env := setupRelayApp(t)
	path := fmt.Sprintf("/api/token/%d/verify-password", env.tokenID)
	before := middleware.Sessions.ListByUser(env.rootID, env.session)
	send := func(session, origin, body string) (int, http.Header, string) {
		t.Helper()
		req, _ := http.NewRequest(http.MethodPost, env.base+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if session != "" {
			req.AddCookie(&http.Cookie{Name: "session", Value: session})
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		return res.StatusCode, res.Header, string(raw)
	}
	body := fmt.Sprintf(`{"expected_user_id":%d,"password":"123456"}`, env.rootID)
	for _, item := range []struct {
		session, origin, body string
		status                int
	}{{"", "", body, 401}, {env.session, "https://other.example.invalid", body, 403}, {env.session, "", body, 200}, {env.session, "", fmt.Sprintf(`{"expected_user_id":%d,"password":"wrong"}`, env.rootID), 403}} {
		status, headers, response := send(item.session, item.origin, item.body)
		if status != item.status || len(headers.Values("Set-Cookie")) != 0 || !strings.Contains(headers.Get("Cache-Control"), "no-store") {
			t.Fatalf("router verification changed session/cache behavior: %d %s", status, response)
		}
		if strings.Contains(response, env.apiKey) || strings.Contains(response, "123456") || strings.Contains(response, "\"session\"") {
			t.Fatal("verification returned credentials or a new session")
		}
	}
	if len(middleware.Sessions.ListByUser(env.rootID, env.session)) != len(before) {
		t.Fatal("verification added a login device")
	}
	hash, err := common.HashPassword("second-password")
	if err != nil {
		t.Fatal(err)
	}
	other := &model.User{Username: "password-other-owner", Password: hash, Status: 1, Role: 1, Group: "default", AffCode: common.GenerateAffCode()}
	if err := model.CreateUser(other); err != nil {
		t.Fatal(err)
	}
	otherToken := &model.Token{UserID: other.ID, Name: "private", Status: 1, Group: "default"}
	if err := model.InsertToken(otherToken); err != nil {
		t.Fatal(err)
	}
	path = fmt.Sprintf("/api/token/%d/verify-password", otherToken.ID)
	if status, _, _ := send(env.session, "", body); status != 404 {
		t.Fatal("Root personal verification bypassed another token's owner")
	}
	if status, _, _ := send(env.session, "", fmt.Sprintf(`{"expected_user_id":%d,"password":"second-password"}`, other.ID)); status != 409 {
		t.Fatal("supplied identity replaced the current session owner")
	}
	middleware.Sessions.Remove(env.session)
	if status, headers, _ := send(env.session, "", body); status != 401 || len(headers.Values("Set-Cookie")) != 0 {
		t.Fatal("expired verification revived a session")
	}
}

func TestTokenPasswordVerificationRateLimitCannotRotateAcrossTokenPaths(t *testing.T) {
	env := setupRelayApp(t)
	blocked := false
	for index := 0; index <= 2*middleware.PasswordVerificationLimitPerMinute; index++ {
		response := doRequest(env.base, http.MethodPost, fmt.Sprintf("/api/token/%d/verify-password", env.tokenID+int64(index)), env.session, map[string]any{})
		if response.Status == 429 {
			blocked = true
			break
		}
		if response.Status != 400 {
			t.Fatalf("unexpected limited request result: %d %s", response.Status, response.Body)
		}
	}
	if !blocked {
		t.Fatal("changing target token IDs bypassed the owner-wide verification limit")
	}
	self := doRequest(env.base, http.MethodGet, "/api/user/self", env.session, nil)
	if self.Status != 200 {
		t.Fatal("verification throttling revoked the existing login session")
	}
}

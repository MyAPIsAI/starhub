package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"starhub/middleware"
)

func TestGuardedDeviceRoutesRejectStaleCookieOwnersAndKeepOtherAccountsActive(t *testing.T) {
	env := setupRelayApp(t)
	aliceID, alice := exportTestUser(t, env.base, "guarded-device-alice")
	bobID, bob := exportTestUser(t, env.base, "guarded-device-bob")
	middleware.Sessions.RemoveAllExcept(aliceID, alice)
	middleware.Sessions.RemoveAllExcept(bobID, bob)
	aliceOther := middleware.Sessions.Create(aliceID, 2*time.Hour)
	bobOther := middleware.Sessions.Create(bobID, 3*time.Hour)
	defer middleware.Sessions.Remove(aliceOther)
	defer middleware.Sessions.Remove(bobOther)
	request := func(method, path, cookie, body string) apiResult {
		t.Helper()
		req, _ := http.NewRequest(method, env.base+path, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: "session", Value: cookie})
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		if len(res.Cookies()) != 0 || !strings.Contains(res.Header.Get("Cache-Control"), "no-store") {
			t.Fatal("device management wrote a login cookie or cacheable private response")
		}
		return apiResult{Status: res.StatusCode, Body: string(raw)}
	}
	statePath := fmt.Sprintf("/api/user/session-state?expected_user_id=%d", aliceID)
	response := request(http.MethodGet, statePath, alice, "")
	var state struct {
		Data struct {
			Schema   string                   `json:"schema"`
			Guarded  bool                     `json:"guarded_management"`
			UserID   int64                    `json:"user_id"`
			Sessions []middleware.SessionInfo `json:"sessions"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(response.Body), &state) != nil || response.Status != 200 || state.Data.Schema != "session_state_v1" || !state.Data.Guarded || state.Data.UserID != aliceID || len(state.Data.Sessions) != 2 {
		t.Fatalf("wrong guarded device state: %d %s", response.Status, response.Body)
	}
	for _, secret := range []string{alice, bob, aliceOther, bobOther} {
		if strings.Contains(response.Body, secret) {
			t.Fatal("private session credential escaped into device metadata")
		}
	}
	for _, cookie := range []string{bob, env.session} {
		if response := request(http.MethodGet, statePath, cookie, ""); response.Status != 409 {
			t.Fatal("a different current cookie read the expected owner's sessions")
		}
		if response := request(http.MethodPost, "/api/user/sessions/terminate-others", cookie, fmt.Sprintf(`{"expected_user_id":%d}`, aliceID)); response.Status != 409 {
			t.Fatal("stale expected owner revoked sessions using another account's cookie")
		}
		for _, secret := range []string{alice, aliceOther, bob, bobOther} {
			if _, ok := middleware.Sessions.Get(secret); !ok {
				t.Fatal("rejected owner changed active sessions")
			}
		}
	}
	if response := request(http.MethodGet, statePath, "", ""); response.Status != 401 {
		t.Fatal("anonymous guarded device read succeeded")
	}
	if response := request(http.MethodGet, statePath+"&expected_user_id=1", alice, ""); response.Status != 400 {
		t.Fatal("duplicate expected owner was accepted")
	}
	for index := 0; index < 2; index++ {
		response := request(http.MethodPost, "/api/user/sessions/terminate-others", alice, fmt.Sprintf(`{"expected_user_id":%d}`, aliceID))
		var receipt struct {
			Data struct {
				UserID     int64 `json:"user_id"`
				Terminated int   `json:"terminated"`
			} `json:"data"`
		}
		if json.Unmarshal([]byte(response.Body), &receipt) != nil || response.Status != 200 || receipt.Data.UserID != aliceID || receipt.Data.Terminated != 1-index {
			t.Fatalf("wrong owner/count receipt: %d %s", response.Status, response.Body)
		}
	}
	for _, secret := range []string{alice, bob, bobOther} {
		if _, ok := middleware.Sessions.Get(secret); !ok {
			t.Fatal("guarded termination removed current or unrelated owner sessions")
		}
	}
	if _, ok := middleware.Sessions.Get(aliceOther); ok {
		t.Fatal("another owned session was not terminated")
	}
	if response := request(http.MethodGet, "/api/user/sessions", alice, ""); response.Status != 200 {
		t.Fatal("legacy device array endpoint regressed")
	}
}

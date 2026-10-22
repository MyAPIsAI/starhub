package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"starhub/middleware"
)

func TestTerminateOtherSessionsPreservesCurrentAndOtherUsers(t *testing.T) {
	env := setupRelayApp(t)
	t.Cleanup(func() { middleware.Sessions.Remove(env.session) })
	aliceID, current := exportTestUser(t, env.base, "session-alice")
	// Clear stale sessions for reused fixture IDs before creating the second login.
	middleware.Sessions.RemoveAllExcept(aliceID, current)
	bobID, bobSession := exportTestUser(t, env.base, "session-bob")
	res := doRequest(env.base, http.MethodPost, "/api/user/login", "", map[string]string{"username": "session-alice", "password": "export-password"})
	var login struct {
		Success bool `json:"success"`
		Data    struct {
			Session string `json:"session"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(res.Body), &login); err != nil || res.Status != http.StatusOK || !login.Success || login.Data.Session == "" || login.Data.Session == current {
		t.Fatalf("second login failed: %d %s (decode error %v)", res.Status, res.Body, err)
	}
	other := login.Data.Session
	t.Cleanup(func() { middleware.Sessions.Remove(other) })
	for _, step := range []struct {
		name  string
		count int
	}{
		{"first termination", 1},
		{"repeat termination", 0},
	} {
		t.Run(step.name, func(t *testing.T) {
			res := doRequest(env.base, http.MethodPost, "/api/user/sessions/terminate-others", current, nil)
			var terminated struct {
				Success bool `json:"success"`
				Data    struct {
					Terminated *int `json:"terminated"`
				} `json:"data"`
			}
			if err := json.Unmarshal([]byte(res.Body), &terminated); err != nil || res.Status != http.StatusOK || !terminated.Success || terminated.Data.Terminated == nil || *terminated.Data.Terminated != step.count {
				t.Fatalf("terminate others: want count %d, got %d %s (decode error %v)", step.count, res.Status, res.Body, err)
			}
			cases := []struct {
				name    string
				session string
				userID  int64
			}{
				{"current session", current, aliceID},
				{"terminated session", other, 0},
				{"other user", bobSession, bobID},
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					res := doRequest(env.base, http.MethodGet, "/api/user/self", tc.session, nil)
					var got struct {
						Success bool   `json:"success"`
						Message string `json:"message"`
						Data    *struct {
							ID int64 `json:"id"`
						} `json:"data"`
					}
					if err := json.Unmarshal([]byte(res.Body), &got); err != nil {
						t.Fatalf("invalid self response: %s (decode error %v)", res.Body, err)
					}
					if tc.userID == 0 {
						if res.Status != http.StatusUnauthorized || got.Success || got.Message != "login required" || got.Data != nil {
							t.Fatalf("terminated session remained usable: %d %s", res.Status, res.Body)
						}
					} else if res.Status != http.StatusOK || !got.Success || got.Data == nil || got.Data.ID != tc.userID {
						t.Fatalf("valid session was affected: %d %s", res.Status, res.Body)
					}
				})
			}
		})
	}
}

func TestSessionListUserIsolation(t *testing.T) {
	env := setupRelayApp(t)
	t.Cleanup(func() { middleware.Sessions.Remove(env.session) })
	aliceID, aliceSession := exportTestUser(t, env.base, "session-alice")
	bobID, bobSession := exportTestUser(t, env.base, "session-bob")
	middleware.Sessions.RemoveAllExcept(aliceID, aliceSession)
	middleware.Sessions.RemoveAllExcept(bobID, bobSession)
	res := doRequest(env.base, http.MethodPost, "/api/user/login", "", map[string]string{"username": "session-alice", "password": "export-password"})
	var login struct {
		Success bool `json:"success"`
		Data    struct {
			Session string `json:"session"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(res.Body), &login); err != nil || res.Status != http.StatusOK || !login.Success || login.Data.Session == "" || login.Data.Session == aliceSession {
		t.Fatalf("second login failed: %d %s (decode error %v)", res.Status, res.Body, err)
	}
	other := login.Data.Session
	t.Cleanup(func() { middleware.Sessions.Remove(other) })
	cases := []struct {
		name    string
		session string
		count   int
	}{
		{"alice", aliceSession, 2},
		{"bob", bobSession, 1},
		{"anonymous", "", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := doRequest(env.base, http.MethodGet, "/api/user/sessions", tc.session, nil)
			var got struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
				Data    []struct {
					CreatedAt *int64 `json:"created_at"`
					ExpiresAt *int64 `json:"expires_at"`
					IsCurrent *bool  `json:"is_current"`
				} `json:"data"`
			}
			if err := json.Unmarshal([]byte(res.Body), &got); err != nil {
				t.Fatalf("invalid session list: %s (decode error %v)", res.Body, err)
			}
			if tc.session == "" {
				if res.Status != http.StatusUnauthorized || got.Success || got.Message != "login required" || got.Data != nil {
					t.Fatalf("unexpected anonymous response: %d %s", res.Status, res.Body)
				}
			} else {
				if res.Status != http.StatusOK || !got.Success || len(got.Data) != tc.count {
					t.Fatalf("want %d owned sessions, got %d %s", tc.count, res.Status, res.Body)
				}
				currentCount := 0
				for _, info := range got.Data {
					if info.CreatedAt == nil || info.ExpiresAt == nil || info.IsCurrent == nil || *info.CreatedAt <= 0 || *info.ExpiresAt <= *info.CreatedAt {
						t.Fatalf("missing or invalid session metadata: %s", res.Body)
					}
					if *info.IsCurrent {
						currentCount++
					}
				}
				if currentCount != 1 {
					t.Fatalf("current session count = %d, want 1", currentCount)
				}
			}
			for credential, userID := range map[string]int64{env.session: env.rootID, aliceSession: aliceID, other: aliceID, bobSession: bobID} {
				if strings.Contains(res.Body, credential) {
					t.Fatal("session list leaked a session credential")
				}
				if gotID, ok := middleware.Sessions.Get(credential); !ok || gotID != userID {
					t.Fatalf("listing changed session ownership or validity for user %d", userID)
				}
			}
		})
	}
}

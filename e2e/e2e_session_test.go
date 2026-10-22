package main

import (
	"encoding/json"
	"net/http"
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

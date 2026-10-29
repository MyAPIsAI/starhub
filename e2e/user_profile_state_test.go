package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestProfileStateRouteRejectsOtherCookiesAndRemainsPrivate(t *testing.T) {
	env := setupRelayApp(t)
	owner, session := exportTestUser(t, env.base, "profile-route-owner")
	_, other := exportTestUser(t, env.base, "profile-route-other")
	path := fmt.Sprintf("/api/user/profile-state?expected_user_id=%d", owner)
	for _, test := range []struct {
		cookie string
		status int
	}{{session, 200}, {other, 409}, {env.session, 409}, {"", 401}} {
		req, _ := http.NewRequest(http.MethodGet, env.base+path, nil)
		if test.cookie != "" {
			req.AddCookie(&http.Cookie{Name: "session", Value: test.cookie})
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != test.status || len(res.Cookies()) != 0 || !strings.Contains(res.Header.Get("Cache-Control"), "no-store") {
			t.Fatalf("unexpected profile route response: %d %s", res.StatusCode, raw)
		}
		if test.status == 200 {
			var got struct {
				Data struct {
					Schema string `json:"schema"`
					User   struct {
						ID       int64  `json:"id"`
						Username string `json:"username"`
					} `json:"user"`
				} `json:"data"`
			}
			if json.Unmarshal(raw, &got) != nil || got.Data.Schema != "profile_state_v1" || got.Data.User.ID != owner || got.Data.User.Username != "profile-route-owner" {
				t.Fatal("wrong profile owner")
			}
		}
		if strings.Contains(string(raw), session) || strings.Contains(string(raw), "password") {
			t.Fatal("profile response exposed credentials")
		}
	}
}

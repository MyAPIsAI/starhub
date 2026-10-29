package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"starhub/model"
)

func TestUserSecurityHistoryRouteIsOwnedAndPrivate(t *testing.T) {
	env := setupRelayApp(t)
	owner, session := exportTestUser(t, env.base, "history-route-owner")
	_, other := exportTestUser(t, env.base, "history-route-other")
	snapshot, _ := model.GetUserByID(owner)
	if err := model.UpdateOwnedProfile(snapshot, "email", "private-history@example.test"); err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/user/security-state?expected_user_id=%d", owner)
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
			t.Fatalf("invalid history route response: %d %s", res.StatusCode, raw)
		}
		if test.status == 200 {
			var state struct {
				Data struct {
					Owner  int64                     `json:"user_id"`
					Scope  string                    `json:"history_scope"`
					Events []model.UserSecurityEvent `json:"events"`
				} `json:"data"`
			}
			if json.Unmarshal(raw, &state) != nil || state.Data.Owner != owner || state.Data.Scope != "guarded_profile_changes_v1" || len(state.Data.Events) != 1 {
				t.Fatal("invalid real owned history")
			}
		}
		for _, secret := range []string{session, other, snapshot.Password, "private-history@example.test"} {
			if strings.Contains(string(raw), secret) {
				t.Fatal("security history exposed contact or credential material")
			}
		}
	}
}

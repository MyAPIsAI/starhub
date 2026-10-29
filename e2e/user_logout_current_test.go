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

func TestLogoutCurrentRouteLeavesBrowserCookiesAndOtherDevicesUntouched(t *testing.T) {
	env := setupRelayApp(t)
	owner, session := exportTestUser(t, env.base, "logout-owned-http")
	_, other := exportTestUser(t, env.base, "logout-other-http")
	same := middleware.Sessions.Create(owner, time.Hour)
	defer middleware.Sessions.Remove(same)
	body := fmt.Sprintf(`{"schema":"logout_current_v1","expected_user_id":%d}`, owner)
	request := func(cookie, origin string) apiResult {
		t.Helper()
		req, _ := http.NewRequest(http.MethodPost, env.base+"/api/user/logout-current", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: "session", Value: cookie})
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
		if len(res.Cookies()) != 0 || !strings.Contains(res.Header.Get("Cache-Control"), "no-store") {
			t.Fatal("logout response changed a browser cookie or allowed private caching")
		}
		return apiResult{Status: res.StatusCode, Body: string(raw)}
	}
	for _, test := range []struct {
		cookie, origin string
		status         int
	}{{"", "", 401}, {other, "", 409}, {env.session, "", 409}, {session, "https://foreign.invalid", 403}} {
		if result := request(test.cookie, test.origin); result.Status != test.status {
			t.Fatalf("unexpected logout guard: %d %s", result.Status, result.Body)
		}
	}
	for _, token := range []string{session, same, other} {
		if _, ok := middleware.Sessions.Get(token); !ok {
			t.Fatal("rejected logout changed active sessions")
		}
	}
	result := request(session, "")
	var receipt struct {
		Data struct {
			Schema string `json:"schema"`
			Owner  int64  `json:"user_id"`
			Out    bool   `json:"signed_out"`
		} `json:"data"`
	}
	if result.Status != 200 || json.Unmarshal([]byte(result.Body), &receipt) != nil || receipt.Data.Schema != "logout_receipt_v1" || receipt.Data.Owner != owner || !receipt.Data.Out {
		t.Fatal("native logout receipt not confirmed")
	}
	if result := request(session, ""); result.Status != 401 {
		t.Fatal("revoked session was revived")
	}
	for _, token := range []string{same, other} {
		if _, ok := middleware.Sessions.Get(token); !ok {
			t.Fatal("other device/account was revoked")
		}
	}
}

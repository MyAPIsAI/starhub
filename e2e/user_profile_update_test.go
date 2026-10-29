package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
)

func TestGuardedProfileUpdateRouteChecksCookieCSRFAndRateLimit(t *testing.T) {
	env := setupRelayApp(t)
	owner, session := exportTestUser(t, env.base, "profile-write-http")
	_, other := exportTestUser(t, env.base, "profile-write-other")
	middleware.ResetRateLimits()
	body := fmt.Sprintf(`{"expected_user_id":%d,"expected_email":"","email":"profile@example.test"}`, owner)
	request := func(cookie, origin, body string) apiResult {
		t.Helper()
		req, _ := http.NewRequest(http.MethodPut, env.base+"/api/user/profile-state", strings.NewReader(body))
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
			t.Fatal("profile update wrote a cookie or allowed caching")
		}
		return apiResult{Status: res.StatusCode, Body: string(raw)}
	}
	for _, test := range []struct {
		cookie, origin string
		status         int
	}{{"", "", 401}, {other, "", 409}, {env.session, "", 409}, {session, "https://foreign.invalid", 403}} {
		if response := request(test.cookie, test.origin, body); response.Status != test.status {
			t.Fatalf("unexpected guard response: %d %s", response.Status, response.Body)
		}
	}
	response := request(session, "", body)
	var receipt struct {
		Data struct {
			UserID int64  `json:"user_id"`
			Field  string `json:"updated_field"`
		} `json:"data"`
	}
	if response.Status != 200 || json.Unmarshal([]byte(response.Body), &receipt) != nil || receipt.Data.UserID != owner || receipt.Data.Field != "email" {
		t.Fatalf("invalid successful update: %d %s", response.Status, response.Body)
	}
	if id, ok := middleware.Sessions.Get(session); !ok || id != owner {
		t.Fatal("profile update replaced login")
	}
	middleware.ResetRateLimits()
	wrong := fmt.Sprintf(`{"expected_user_id":%d,"expected_email":"profile@example.test","current_password":"wrong","new_password":"replacement-password"}`, owner)
	for i := 0; i < middleware.PasswordVerificationLimitPerMinute; i++ {
		if result := request(session, "", wrong); result.Status != 403 {
			t.Fatalf("wrong password attempt %d: %d", i, result.Status)
		}
	}
	if result := request(session, "", wrong); result.Status != 429 {
		t.Fatalf("owner-wide profile password limiter bypassed: %d", result.Status)
	}
	current, _ := model.GetUserByID(owner)
	if current.Email != "profile@example.test" || !common.VerifyPassword("export-password", current.Password) {
		t.Fatal("failed/throttled attempts changed profile")
	}
}

package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"starhub/common"
	"starhub/config"
	"starhub/middleware"
	"starhub/model"
)

func p6User(t *testing.T, name string, role int) (*model.User, string) {
	t.Helper()
	password, err := common.HashPassword("p6-password")
	if err != nil {
		t.Fatal(err)
	}
	u := &model.User{Username: name, Password: password, Role: role, Status: 1, Group: "default", AffCode: "invite-" + name}
	if err := model.CreateUser(u); err != nil {
		t.Fatal(err)
	}
	return u, middleware.Sessions.Create(u.ID, time.Hour)
}

func TestP6ManagementContract(t *testing.T) {
	env := setupRelayApp(t)
	admin, adminSession := p6User(t, "operator", common.RoleAdmin)
	member, memberSession := p6User(t, "member", common.RoleUser)
	request := func(method, path, session string, body any, want int) apiResult {
		t.Helper()
		res := doRequest(env.base, method, path, session, body)
		if res.Status != want {
			t.Fatalf("%s %s: want %d, got %d %s", method, path, want, res.Status, res.Body)
		}
		return res
	}
	request("GET", "/api/user/", adminSession, nil, 200)
	request("PUT", "/api/user/", adminSession, map[string]any{"id": member.ID, "quota": 100, "expected_quota": 0}, 403)
	var audit model.AuditLog
	if err := model.DB.Order("id DESC").First(&audit).Error; err != nil {
		t.Fatal(err)
	}
	if audit.UserID != admin.ID || audit.Username != "operator" || audit.Status != 403 {
		t.Fatalf("denied actor lost: %+v", audit)
	}
	request("GET", "/api/channel/", memberSession, nil, 403)
	request("PUT", "/api/user/", env.session, map[string]any{"id": member.ID, "quota": 100}, 400)
	request("PUT", "/api/user/", env.session, map[string]any{"id": member.ID, "quota": 100, "expected_quota": 0}, 200)
	if err := model.IncreaseUserQuota(member.ID, 7); err != nil {
		t.Fatal(err)
	}
	request("PUT", "/api/user/", env.session, map[string]any{"id": member.ID, "quota": 500, "expected_quota": 100, "status": 2}, 409)
	u, _ := model.GetUserByID(member.ID)
	if u.Quota != 107 || u.Status != 1 {
		t.Fatalf("conflict changed record: %+v", u)
	}
	request("PUT", "/api/user/", env.session, map[string]any{"id": env.rootID, "status": 2}, 400)
	request("PUT", "/api/user/", env.session, map[string]any{"id": member.ID, "quota": 0, "expected_quota": 107}, 200)
	var last model.AuditLog
	if err := model.DB.Order("id DESC").First(&last).Error; err != nil {
		t.Fatal(err)
	}
	if last.UserID != env.rootID || last.Username != "root" || last.Status != 200 {
		t.Fatalf("successful actor lost: %+v", last)
	}

	created := request("POST", "/api/token/", memberSession, map[string]any{"name": "zero", "remain_quota": 0, "model_limits": "gpt-4o-mini", "allow_ips": "127.0.0.1", "group": "default", "auto_groups": "default"}, 200)
	var token struct {
		Data model.Token `json:"data"`
	}
	if err := json.Unmarshal([]byte(created.Body), &token); err != nil {
		t.Fatal(err)
	}
	stored, _ := model.GetTokenByID(token.Data.ID)
	if stored.RemainQuota != 0 {
		t.Fatalf("explicit zero replaced: %d", stored.RemainQuota)
	}
	request("PUT", "/api/token/", memberSession, map[string]any{"id": stored.ID, "name": "renamed"}, 200)
	stored, _ = model.GetTokenByID(stored.ID)
	if stored.ModelLimits == "" {
		t.Fatal("omitted field was cleared")
	}
	request("PUT", "/api/token/", memberSession, map[string]any{"id": stored.ID, "group": "", "model_limits": "", "allow_ips": "", "auto_groups": ""}, 200)
	stored, _ = model.GetTokenByID(stored.ID)
	if stored.Group != "" || stored.ModelLimits != "" || stored.AllowIps != "" || stored.AutoGroups != "" {
		t.Fatal("explicit empty fields not cleared")
	}
	request("PUT", "/api/token/", memberSession, map[string]any{"id": stored.ID, "remain_quota": 10}, 400)
	request("PUT", "/api/token/", memberSession, map[string]any{"id": stored.ID, "remain_quota": 10, "expected_quota": 0}, 200)
	request("PUT", "/api/token/", memberSession, map[string]any{"id": stored.ID, "remain_quota": 20, "expected_quota": 0, "name": "stale"}, 409)
	request("PUT", "/api/token/", memberSession, map[string]any{"id": env.tokenID, "name": "forbidden"}, 403)
	request("PUT", "/api/token/", adminSession, map[string]any{"id": stored.ID, "unlimited_quota": true, "expected_quota": 10}, 200)
	request("PUT", "/api/token/", adminSession, map[string]any{"id": stored.ID, "unlimited_quota": false, "remain_quota": 0, "expected_quota": 0}, 200)
	stored, _ = model.GetTokenByID(stored.ID)
	if stored.UnlimitedQuota || stored.RemainQuota != 0 {
		t.Fatal("false and zero updates were lost")
	}
	id := createChannelID(t, env, map[string]any{"name": "clear-base", "type": 1, "key": upstreamKey, "models": "gpt-4o-mini", "base_url": "https://example.com"})
	request("PUT", "/api/channel/", env.session, map[string]any{"id": id, "base_url": "", "priority": 0, "weight": 0}, 200)
	channel, _ := model.GetChannelByID(id)
	if channel.BaseURL != "" || channel.Weight != 0 {
		t.Fatal("channel empty/zero update was lost")
	}
	logs := request("GET", "/api/log/?p=-3&page_size=999", env.session, nil, 200)
	var page struct {
		Data struct {
			Page     int `json:"page"`
			PageSize int `json:"page_size"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(logs.Body), &page); err != nil {
		t.Fatal(err)
	}
	if page.Data.Page != 1 || page.Data.PageSize != 100 {
		t.Fatalf("pagination metadata not normalized: %s", logs.Body)
	}
}

func TestP6CookieAndCrossOrigin(t *testing.T) {
	env := setupRelayApp(t)
	if err := config.ApplyStartup(config.Startup{Bootstrap: config.Bootstrap{ForceSecureCookie: true}}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = config.ApplyStartup(config.Startup{}) })
	login, _ := http.NewRequest("POST", env.base+"/api/user/login", strings.NewReader(`{"username":"root","password":"123456"}`))
	login.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(login)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("login response %d %v", resp.StatusCode, resp.Header)
	}
	cookies := resp.Cookies()
	if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie flags: %+v", cookies)
	}
	for _, headers := range []map[string]string{{"Origin": "https://evil.example"}, {"Sec-Fetch-Site": "cross-site"}} {
		req, _ := http.NewRequest("PUT", env.base+"/api/user/", strings.NewReader(`{}`))
		req.AddCookie(cookies[0])
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != 403 || res.Header.Get("Cache-Control") != "no-store" {
			t.Fatalf("cross-origin accepted: %d", res.StatusCode)
		}
	}
	logout, _ := http.NewRequest("POST", env.base+"/api/user/logout", nil)
	logout.AddCookie(cookies[0])
	logout.Header.Set("Origin", env.base)
	resp, err = http.DefaultClient.Do(logout)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 || len(resp.Cookies()) != 1 || resp.Cookies()[0].MaxAge != -1 {
		t.Fatalf("logout did not expire cookie: %v", resp.Header)
	}
	if _, ok := middleware.Sessions.Get(cookies[0].Value); ok {
		t.Fatal("logout kept server session")
	}
}

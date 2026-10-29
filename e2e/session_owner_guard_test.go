package main

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"starhub/model"
)

func TestSessionOwnerHeaderRejectsBeforeLegacyKeyWrites(t *testing.T) {
	env := setupRelayApp(t)
	owner, session := exportTestUser(t, env.base, "header-owner")
	_, other := exportTestUser(t, env.base, "header-other")
	request := func(cookie, method, path string, headers []string) int {
		t.Helper()
		req, _ := http.NewRequest(method, env.base+path, strings.NewReader(`{"name":"guarded-header-key","unlimited_quota":true}`))
		req.Header.Set("Content-Type", "application/json")
		for _, value := range headers {
			req.Header.Add("X-StarHub-Expected-User-ID", value)
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
			t.Fatal("guard changed cookies or permitted caching")
		}
		if res.StatusCode == 200 && (res.Header.Get("X-StarHub-Owner-Guard") != "v1" || res.Header.Get("X-StarHub-User-ID") != strconv.FormatInt(owner, 10)) {
			t.Fatal("missing authenticated owner metadata")
		}
		if (res.StatusCode == 400 || res.StatusCode == 409 || res.StatusCode == 401) && res.Header.Get("X-StarHub-User-ID") != "" {
			t.Fatalf("failed guard exposed an owner: %s", raw)
		}
		return res.StatusCode
	}
	expected := []string{fmt.Sprint(owner)}
	for _, cookie := range []string{other, env.session} {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			if got := request(cookie, method, "/api/token", expected); got != 409 {
				t.Fatalf("foreign cookie passed %s guard: %d", method, got)
			}
		}
	}
	for _, values := range [][]string{{""}, {"0"}, {"-1"}, {"+1"}, {"01"}, {"1,2"}, {"9223372036854775808"}, {fmt.Sprint(owner), fmt.Sprint(owner)}} {
		if got := request(session, http.MethodPost, "/api/token", values); got != 400 {
			t.Fatalf("invalid header %q passed: %d", values, got)
		}
	}
	if got := request("", http.MethodPost, "/api/token", expected); got != 401 {
		t.Fatalf("owner header granted authentication: %d", got)
	}
	var count int64
	if err := model.DB.Model(&model.Token{}).Where("name = ?", "guarded-header-key").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("rejected requests wrote keys: %d %v", count, err)
	}
	if got := request(session, http.MethodGet, "/api/finance/summary", expected); got != 403 {
		t.Fatalf("owner header bypassed role authorization: %d", got)
	}
	for _, values := range [][]string{expected, nil} {
		if got := request(session, http.MethodPost, "/api/token", values); got != 200 {
			t.Fatalf("same-owner or legacy request failed: %d", got)
		}
	}
	if err := model.DB.Model(&model.Token{}).Where("user_id = ? AND name = ?", owner, "guarded-header-key").Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("successful keys have wrong owner: %d %v", count, err)
	}
}

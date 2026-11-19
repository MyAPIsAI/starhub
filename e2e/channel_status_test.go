package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"starhub/common"
	"starhub/model"
)

func TestChannelStatusRouteAuthorizationAndPersistence(t *testing.T) {
	env := setupRelayApp(t)
	id := createChannelID(t, env, map[string]any{
		"name": "availability", "models": "gpt-4o-mini", "key": "first-secret\nsecond-secret",
	})
	path := fmt.Sprintf("/api/channel/%d/status", id)
	set := func(token string, enabled bool) apiResult {
		return doRequest(env.base, http.MethodPut, path, token, map[string]bool{"enabled": enabled})
	}
	if res := set("", false); res.Status != http.StatusUnauthorized {
		t.Fatalf("anonymous availability update: %d", res.Status)
	}
	u := &model.User{Username: "status-viewer", Password: "unused-password", Group: "default", Role: common.RoleUser, Status: 1, AffCode: common.GenerateAffCode()}
	if err := model.CreateUser(u); err != nil {
		t.Fatal(err)
	}
	if res := set(fixtureSession(t, u), false); res.Status != http.StatusForbidden {
		t.Fatalf("ordinary user changed channel availability: %d %s", res.Status, res.Body)
	}
	if _, err := model.DisableChannelKey(id, 0); err != nil {
		t.Fatal(err)
	}
	for _, enabled := range []bool{false, true} {
		res := set(env.session, enabled)
		if res.Status != http.StatusOK || strings.Contains(res.Body, "first-secret") || strings.Contains(res.Body, "second-secret") {
			t.Fatalf("availability response: %d %s", res.Status, res.Body)
		}
		stored, err := model.GetChannelByID(id)
		if err != nil || (stored.Status == common.ChannelStatusEnabled) != enabled {
			t.Fatal("availability was not persisted")
		}
		if stored.MultiKeyStatus != "2,1" {
			t.Fatal("model resource availability changed disabled keys")
		}
		list := doRequest(env.base, http.MethodGet, "/api/channel", env.session, nil)
		if list.Status != http.StatusOK || !strings.Contains(list.Body, fmt.Sprintf(`"status":%d`, stored.Status)) {
			t.Fatal("refreshed channel list did not reflect availability")
		}
	}
	var count int64
	if err := model.DB.Model(&model.AuditLog{}).Where("path = ? AND method = ? AND status = ?", path, http.MethodPut, http.StatusOK).Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("successful availability updates were not audited: %d %v", count, err)
	}
}

func TestChannelResourceRouteAuthorizationAndIsolation(t *testing.T) {
	env := setupRelayApp(t)
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"resource-check","object":"chat.completion","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	t.Cleanup(upstream.Close)
	id := createChannelID(t, env, map[string]any{
		"name": "model availability", "models": "gpt-4o-mini,gpt-4o", "key": upstreamKey, "base_url": upstream.URL,
	})
	path := fmt.Sprintf("/api/channel/%d/resource-status", id)
	set := func(token, name string, enabled bool) apiResult {
		return doRequest(env.base, http.MethodPut, path, token, map[string]any{"model": name, "enabled": enabled})
	}
	if res := set("", "gpt-4o-mini", false); res.Status != http.StatusUnauthorized {
		t.Fatalf("anonymous resource update: %d", res.Status)
	}
	u := &model.User{Username: "resource-viewer", Password: "unused-password", Group: "default", Role: common.RoleUser, Status: 1, AffCode: common.GenerateAffCode()}
	if err := model.CreateUser(u); err != nil {
		t.Fatal(err)
	}
	if res := set(fixtureSession(t, u), "gpt-4o-mini", false); res.Status != http.StatusForbidden {
		t.Fatalf("ordinary user changed resource: %d", res.Status)
	}
	if res := set(env.session, "unknown", true); res.Status != http.StatusNotFound {
		t.Fatalf("unknown resource created: %d %s", res.Status, res.Body)
	}
	check := func(state string, enabled int) {
		t.Helper()
		res := doRequest(env.base, http.MethodGet, fmt.Sprintf("/api/channel/%d", id), env.session, nil)
		var response struct {
			Data struct {
				State model.ChannelResourceState `json:"resource_state"`
			} `json:"data"`
		}
		if json.Unmarshal([]byte(res.Body), &response) != nil || res.Status != 200 || response.Data.State.State != state || response.Data.State.Total != 2 || response.Data.State.Enabled != enabled || strings.Contains(res.Body, upstreamKey) {
			t.Fatalf("refreshed resource summary: %d %s", res.Status, res.Body)
		}
	}
	check("enabled", 2)
	for i, name := range []string{"gpt-4o-mini", "gpt-4o"} {
		if res := set(env.session, name, false); res.Status != http.StatusOK {
			t.Fatalf("disable resource: %d %s", res.Status, res.Body)
		}
		if i == 0 {
			check("partial", 1)
		} else {
			check("closed", 0)
		}
	}
	before, _ := model.GetUserByID(env.rootID)
	res := doRequest(env.base, http.MethodPost, "/v1/chat/completions", env.apiKey, map[string]any{
		"model": "gpt-4o-mini", "messages": []map[string]string{{"role": "user", "content": "hello"}}, "max_tokens": 8,
	})
	after, _ := model.GetUserByID(env.rootID)
	if res.Status != http.StatusServiceUnavailable || calls.Load() != 0 || before.Quota != after.Quota || before.UsedQuota != after.UsedQuota {
		t.Fatalf("disabled resource reached upstream or charged quota: %d %s", res.Status, res.Body)
	}
	if res := set(env.session, "gpt-4o", true); res.Status != http.StatusOK {
		t.Fatalf("enable one of all-disabled resources: %d %s", res.Status, res.Body)
	}
	check("partial", 1)
	discovery := doRequest(env.base, http.MethodGet, "/v1/models", env.apiKey, nil)
	if discovery.Status != 200 || strings.Contains(discovery.Body, `"id":"gpt-4o-mini"`) || !strings.Contains(discovery.Body, `"id":"gpt-4o"`) {
		t.Fatalf("model discovery ignored resource state: %d %s", discovery.Status, discovery.Body)
	}
	res = doRequest(env.base, http.MethodPost, "/v1/chat/completions", env.apiKey, map[string]any{
		"model": "gpt-4o", "messages": []map[string]string{{"role": "user", "content": "hello"}}, "max_tokens": 8,
	})
	if res.Status != 200 || calls.Load() != 1 {
		t.Fatalf("enabled resource unavailable: %d %s", res.Status, res.Body)
	}
	var count int64
	if err := model.DB.Model(&model.AuditLog{}).Where("path = ? AND method = ? AND status = ?", path, http.MethodPut, http.StatusOK).Count(&count).Error; err != nil || count != 3 {
		t.Fatalf("resource writes were not audited: %d %v", count, err)
	}
}

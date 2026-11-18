package main

import (
	"fmt"
	"net/http"
	"strings"
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
		if enabled && strings.Contains(stored.MultiKeyStatus, "2") {
			t.Fatal("explicit enable did not restore disabled resources")
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

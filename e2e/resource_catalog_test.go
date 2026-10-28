package main

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
)

func TestResourceCatalogUsesActualAdminPolicyAndPersistsPlacement(t *testing.T) {
	env := setupRelayApp(t)
	snapshot := model.ResourceSnapshot{ID: "inventory", Sequence: 1, PublishedAt: "2026-10-28T00:00:00Z", Channels: []model.ResourceChannel{{ID: "supplier", Name: "Supplier", Enabled: true}}, Instances: []model.ResourceInstance{{ID: "resource", ChannelID: "supplier", UpstreamModel: "gpt-4o", CanonicalModel: "gpt-4o", Protocols: []string{"OpenAI"}, Capabilities: []string{"text"}, Status: "available", Region: "test-region"}}}
	payload := map[string]any{"expected_revision": 0, "snapshot": snapshot}
	for _, role := range []int{common.RoleUser, common.RoleFinance, 99, 101} {
		user := &model.User{Username: fmt.Sprintf("inventory-role-%d", role), Role: role, Status: common.StatusEnabled, AffCode: common.GenerateAffCode()}
		if err := model.CreateUser(user); err != nil {
			t.Fatal(err)
		}
		session := middleware.Sessions.Create(user.ID, time.Hour)
		t.Cleanup(func() { middleware.Sessions.Remove(session) })
		for _, request := range []struct {
			method, path string
			body         any
		}{{"GET", "/api/resource", nil}, {"GET", "/api/placement", nil}, {"PUT", "/api/resource", payload}, {"PUT", "/api/placement/resource", map[string]any{"expected_revision": 1, "placement": model.ResourcePlacement{GroupIDs: []string{}}}}} {
			if res := doRequest(env.base, request.method, request.path, session, request.body); res.Status != 403 {
				t.Fatalf("role %d reached inventory: %d %s", role, res.Status, res.Body)
			}
		}
	}
	if res := doRequest(env.base, "GET", "/api/resource", "", nil); res.Status != 401 {
		t.Fatalf("anonymous read: %d %s", res.Status, res.Body)
	}
	decode := func(res apiResult) model.ResourceCatalog {
		t.Helper()
		if res.Status != 200 {
			t.Fatalf("request failed: %d %s", res.Status, res.Body)
		}
		var body struct {
			Success bool                  `json:"success"`
			Data    model.ResourceCatalog `json:"data"`
		}
		if err := json.Unmarshal([]byte(res.Body), &body); err != nil || !body.Success {
			t.Fatalf("bad envelope: %v %s", err, res.Body)
		}
		return body.Data
	}
	state := decode(doRequest(env.base, "PUT", "/api/resource", env.session, payload))
	if state.Revision != 1 || state.Placements["resource"].Enabled || state.AffectsGatewayRouting {
		t.Fatalf("import changed authorization: %+v", state)
	}
	admin := &model.User{Username: "inventory-admin", Role: common.RoleAdmin, Status: common.StatusEnabled, AffCode: common.GenerateAffCode()}
	if err := model.CreateUser(admin); err != nil {
		t.Fatal(err)
	}
	session := middleware.Sessions.Create(admin.ID, time.Hour)
	t.Cleanup(func() { middleware.Sessions.Remove(session) })
	standard := "gpt-4o"
	placement := model.ResourcePlacement{ModelID: &standard, GroupIDs: []string{"default"}, Enabled: true}
	state = decode(doRequest(env.base, "PUT", "/api/placement/resource", session, map[string]any{"expected_revision": 1, "placement": placement}))
	if state.Revision != 2 || !state.Placements["resource"].Enabled {
		t.Fatalf("placement not persisted: %+v", state)
	}
	if res := doRequest(env.base, "PUT", "/api/resource", env.session, payload); res.Status != 409 {
		t.Fatalf("stale edit not rejected: %d %s", res.Status, res.Body)
	}
	read := decode(doRequest(env.base, "GET", "/api/placement", session, nil))
	if read.Revision != 2 || read.Snapshot.Instances[0].Region != "test-region" || !read.Placements["resource"].Enabled {
		t.Fatalf("read lost persisted data: %+v", read)
	}
	var audits int64
	if err := model.DB.Model(&model.AuditLog{}).Where("path IN ? AND status = 200", []string{"/api/resource", "/api/placement/resource"}).Count(&audits).Error; err != nil || audits != 2 {
		t.Fatalf("inventory audit: %d %v", audits, err)
	}
	if err := model.UpdateUserFields(admin.ID, map[string]any{"role": common.RoleUser}); err != nil {
		t.Fatal(err)
	}
	if res := doRequest(env.base, "GET", "/api/resource", session, nil); res.Status != 403 {
		t.Fatalf("revoked admin retained inventory access: %d %s", res.Status, res.Body)
	}
}

package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
)

func TestResourceRegionRoutesUseAdminPolicyAndRevision(t *testing.T) {
	env := setupRelayApp(t)
	for _, role := range []int{common.RoleUser, common.RoleFinance, common.RoleAdmin, 99, 101} {
		user := &model.User{Username: fmt.Sprintf("region-role-%d", role), Role: role, Status: common.StatusEnabled, AffCode: common.GenerateAffCode()}
		if err := model.CreateUser(user); err != nil {
			t.Fatal(err)
		}
		session := middleware.Sessions.Create(user.ID, time.Hour)
		t.Cleanup(func() { middleware.Sessions.Remove(session) })
		want := 403
		if role == common.RoleAdmin {
			want = 200
		}
		for _, request := range []struct {
			method, path string
			payload      any
		}{{"GET", "/api/region", nil}, {"PUT", "/api/region/test-region", map[string]any{"expected_revision": 0, "name": "Test region", "description": "Inventory metadata"}}, {"DELETE", "/api/region/test-region", map[string]any{"expected_revision": 1}}} {
			res := doRequest(env.base, request.method, request.path, session, request.payload)
			if res.Status != want {
				t.Fatalf("role %d %s %s: %d %s", role, request.method, request.path, res.Status, res.Body)
			}
		}
	}
	for _, request := range []struct {
		method, path string
		payload      any
		want         int
	}{{"PUT", "/api/region/root-region", map[string]any{"expected_revision": 2, "name": "Root region"}, 200}, {"PUT", "/api/region/root-region", map[string]any{"expected_revision": 2, "name": "Stale rename"}, 409}, {"DELETE", "/api/region/root-region", map[string]any{}, 400}} {
		res := doRequest(env.base, request.method, request.path, env.session, request.payload)
		if res.Status != request.want {
			t.Fatalf("root %s: %d %s", request.path, res.Status, res.Body)
		}
	}
	state, err := model.ReadResourceCatalog(context.Background())
	if err != nil || state.Revision != 3 || state.Regions["root-region"].Name != "Root region" || state.AffectsGatewayRouting {
		t.Fatalf("region state: %+v %v", state, err)
	}
	var audits int64
	if err := model.DB.Model(&model.AuditLog{}).Where("path LIKE '/api/region/%' AND status = 200").Count(&audits).Error; err != nil || audits != 3 {
		t.Fatalf("region audits: %d %v", audits, err)
	}
}

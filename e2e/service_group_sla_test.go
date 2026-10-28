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

func TestServiceGroupTargetsUseActualPolicyAndAuditWithoutPricingWrites(t *testing.T) {
	env := setupRelayApp(t)
	group := &model.ServiceGroupConfig{GroupID: "target-group", Name: "Target", Multiplier: "0.8", Enabled: true}
	if err := model.UpsertServiceGroup(group); err != nil {
		t.Fatal(err)
	}
	before, err := model.OptionVersion()
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/service-group/target-group/sla"
	payload := map[string]any{"expected_group_id": group.ID, "expected_revision": 0, "targets": map[string]any{"success_rate_ppm": 995000, "p95_latency_ms": 800}}
	for _, role := range []int{common.RoleUser, common.RoleFinance, common.RoleAdmin, 99, 101} {
		user := &model.User{Username: fmt.Sprintf("sla-role-%d", role), Role: role, Status: common.StatusEnabled, AffCode: common.GenerateAffCode()}
		if err := model.CreateUser(user); err != nil {
			t.Fatal(err)
		}
		session := middleware.Sessions.Create(user.ID, time.Hour)
		t.Cleanup(func() { middleware.Sessions.Remove(session) })
		want := 403
		if role == common.RoleAdmin {
			want = 200
		}
		if res := doRequest(env.base, "PUT", path, session, payload); res.Status != want {
			t.Fatalf("role %d target write: %d %s", role, res.Status, res.Body)
		}
		if res := doRequest(env.base, "GET", "/api/service-group", session, nil); res.Status != want {
			t.Fatalf("role %d group read: %d %s", role, res.Status, res.Body)
		}
		if role != common.RoleAdmin {
			if res := doRequest(env.base, "PUT", "/api/service-group", session, map[string]any{"group_id": group.GroupID, "name": "Unauthorized", "multiplier": "2"}); res.Status != 403 {
				t.Fatalf("role %d bypassed group policy: %d %s", role, res.Status, res.Body)
			}
		}
	}
	if res := doRequest(env.base, "PUT", path, env.session, payload); res.Status != 409 {
		t.Fatalf("stale target write: %d %s", res.Status, res.Body)
	}
	payload["expected_revision"] = 1
	payload["targets"] = map[string]any{"p95_latency_ms": 1000}
	if res := doRequest(env.base, "PUT", path, env.session, payload); res.Status != 200 {
		t.Fatalf("root publish: %d %s", res.Status, res.Body)
	}
	payload["expected_revision"] = 2
	payload["targets"] = nil
	res := doRequest(env.base, "PUT", path, env.session, payload)
	var reply struct {
		Data struct {
			ID         int64                 `json:"id"`
			Multiplier string                `json:"multiplier"`
			SLA        model.ServiceGroupSLA `json:"sla"`
		} `json:"data"`
	}
	if res.Status != 200 {
		t.Fatalf("root unpublish: %d %s", res.Status, res.Body)
	}
	if err := json.Unmarshal([]byte(res.Body), &reply); err != nil || reply.Data.ID != group.ID || reply.Data.Multiplier != "0.8" || reply.Data.SLA.Revision != 3 || reply.Data.SLA.Targets != nil {
		t.Fatalf("unpublish response: %s %v", res.Body, err)
	}
	if after, _ := model.OptionVersion(); after != before {
		t.Fatal("target edit changed pricing configuration")
	}
	var audits int64
	if err := model.DB.Model(&model.AuditLog{}).Where("path = ? AND status = 200", path).Count(&audits).Error; err != nil || audits != 3 {
		t.Fatalf("target audit: %d %v", audits, err)
	}
}

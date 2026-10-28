package main

import (
	"fmt"
	"testing"
	"time"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
)

func TestFinanceRoleAssignmentIsRootOnlyAndRevocable(t *testing.T) {
	env := setupRelayApp(t)
	user := &model.User{Username: "finance-target", Role: common.RoleUser, Status: common.StatusEnabled, Group: "default", Quota: 1000, AffCode: common.GenerateAffCode()}
	if err := model.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	userSession := middleware.Sessions.Create(user.ID, time.Hour)
	t.Cleanup(func() { middleware.Sessions.Remove(userSession) })
	path := fmt.Sprintf("/api/user/%d/finance-role", user.ID)
	body := map[string]any{"enabled": true, "expected_role": common.RoleUser}
	if res := doRequest(env.base, "PUT", path, userSession, body); res.Status != 403 {
		t.Fatalf("self-assigned finance: %d %s", res.Status, res.Body)
	}
	if res := doRequest(env.base, "PUT", path, env.session, body); res.Status != 200 {
		t.Fatalf("root grant failed: %d %s", res.Status, res.Body)
	}
	got, err := model.GetUserByID(user.ID)
	if err != nil || got.Role != common.RoleFinance || got.IsAdmin() {
		t.Fatalf("finance became admin: %+v %v", got, err)
	}
	for _, api := range []string{"/api/user", "/api/channel", "/api/option", "/api/audit"} {
		if res := doRequest(env.base, "GET", api, userSession, nil); res.Status != 403 {
			t.Fatalf("finance read admin API %s: %d %s", api, res.Status, res.Body)
		}
	}
	if res := doRequest(env.base, "PUT", path, userSession, map[string]any{"enabled": false, "expected_role": common.RoleFinance}); res.Status != 403 {
		t.Fatalf("finance changed its own role: %d %s", res.Status, res.Body)
	}
	if res := doRequest(env.base, "PUT", path, env.session, map[string]any{"enabled": false, "expected_role": common.RoleFinance}); res.Status != 200 {
		t.Fatalf("root revoke failed: %d %s", res.Status, res.Body)
	}
	got, _ = model.GetUserByID(user.ID)
	if got.Role != common.RoleUser || got.Quota != 1000 {
		t.Fatalf("revocation changed quota or retained role: %+v", got)
	}
	var audits []model.AuditLog
	if err := model.DB.Where("path = ? AND user_id = ? AND status = ?", path, env.rootID, 200).Find(&audits).Error; err != nil || len(audits) != 2 {
		t.Fatalf("role changes not audited with target path: %+v %v", audits, err)
	}
}

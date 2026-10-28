package main

import (
	"fmt"
	"testing"
	"time"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
	"starhub/setting"
)

func TestOptionReloadUsesAdminPolicyAndAudit(t *testing.T) {
	env := setupRelayApp(t)
	before := setting.ModelRatioValue("gpt-4o")
	version, err := model.SaveOption(setting.OptionKeyModelRatio, `{"gpt-4o":77}`, env.rootID)
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []int{common.RoleUser, common.RoleFinance, common.RoleAdmin} {
		user := &model.User{Username: fmt.Sprintf("reload-%d", role), Role: role, Status: common.StatusEnabled, AffCode: common.GenerateAffCode()}
		if err := model.CreateUser(user); err != nil {
			t.Fatal(err)
		}
		session := middleware.Sessions.Create(user.ID, time.Hour)
		t.Cleanup(func() { middleware.Sessions.Remove(session) })
		res := doRequest(env.base, "POST", "/api/option/reload", session, nil)
		if role == common.RoleAdmin {
			if res.Status != 200 || model.LoadedOptionVersion() != version || setting.ModelRatioValue("gpt-4o") != 77 {
				t.Fatalf("admin reload failed: %d %s", res.Status, res.Body)
			}
		} else if res.Status != 403 || setting.ModelRatioValue("gpt-4o") != before {
			t.Fatalf("role %d changed configuration: %d %s", role, res.Status, res.Body)
		}
	}
	if res := doRequest(env.base, "POST", "/api/option/reload", env.session, nil); res.Status != 200 {
		t.Fatalf("root reload failed: %d %s", res.Status, res.Body)
	}
	var count int64
	if err := model.DB.Model(&model.AuditLog{}).Where("path = ? AND status = ?", "/api/option/reload", 200).Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("reload audit count=%d %v", count, err)
	}
}

package model

import (
	"starhub/common"
	"testing"
)

func TestStaffOperatorInitializesOnlyTheFirstRoot(t *testing.T) {
	newTestDB(t)
	if err := OperateStaffAccount("initialize", "first-root", "LongOperatorSecret92", "operator", "Initial installation"); err != nil {
		t.Fatal(err)
	}
	root, err := GetUserByUsername("first-root")
	if err != nil || root.Role != common.RoleRoot || root.AdminPasswordChangedAt != 0 || !common.VerifyPassword("LongOperatorSecret92", root.Password) {
		t.Fatal("incorrect initialized root")
	}
	if err := OperateStaffAccount("initialize", "second-root", "LongOperatorSecret92", "operator", "Retry initialization"); err == nil {
		t.Fatal("created a second root")
	}
}

func TestStaffOperatorResetIsAuditedAndRevokesOldGeneration(t *testing.T) {
	newTestDB(t)
	user := &User{Username: "operator-staff", Role: common.RoleAdmin, Status: 1, AffCode: common.GenerateAffCode()}
	if err := CreateUser(user); err != nil {
		t.Fatal(err)
	}
	if err := DB.Create(&TwoFA{UserID: user.ID, Secret: "factor", IsEnabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := OperateStaffAccount("reset-2fa", user.Username, "", "operator", "Lost authenticator"); err != nil {
		t.Fatal(err)
	}
	fresh, err := GetUserByID(user.ID)
	if err != nil || fresh.DeactivationVersion != user.DeactivationVersion+1 {
		t.Fatal("session generation not revoked")
	}
	if _, err := GetTwoFA(user.ID); err != Err2FANotEnabled {
		t.Fatal("factor not reset")
	}
	var audit AuditLog
	if err := DB.Where("path = ? AND user_id = ?", "/operator/reset-2fa", user.ID).First(&audit).Error; err != nil || audit.Reason != "Lost authenticator" || audit.Username != "operator" {
		t.Fatalf("missing audit: %v", err)
	}
	if err := DB.Create(&TwoFA{UserID: user.ID, Secret: "retained", IsEnabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.Exec(`CREATE TRIGGER reject_operator_audit BEFORE INSERT ON audit_logs BEGIN SELECT RAISE(ABORT, 'audit failed'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if err := OperateStaffAccount("reset-2fa", user.Username, "", "operator", "Retry"); err == nil {
		t.Fatal("audit failure was ignored")
	}
	factor, err := GetTwoFA(user.ID)
	if err != nil || !factor.IsEnabled || factor.Secret != "retained" {
		t.Fatal("audit failure did not roll back reset")
	}
}

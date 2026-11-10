package model

import (
	"testing"

	"starhub/common"
)

func TestSmartAccessNilAllowsSmartRouting(t *testing.T) {
	tok := &Token{ModelLimits: ""}
	if !tok.AllowsSmartRouting() {
		t.Fatal("nil SmartAccess should allow smart routing (backward compat)")
	}
}

func TestSmartAccessFalseDisablesSmartRouting(t *testing.T) {
	f := false
	tok := &Token{ModelLimits: "gpt-4o", SmartAccess: &f}
	if tok.AllowsSmartRouting() {
		t.Fatal("SmartAccess=false should disable smart routing")
	}
}

func TestSmartAccessTrueEnablesSmartRouting(t *testing.T) {
	tr := true
	tok := &Token{ModelLimits: "gpt-4o", SmartAccess: &tr}
	if !tok.AllowsSmartRouting() {
		t.Fatal("SmartAccess=true should enable smart routing")
	}
}

func TestSmartAccessFalseOverridesEmptyModelLimits(t *testing.T) {
	f := false
	tok := &Token{ModelLimits: "", SmartAccess: &f}
	if tok.AllowsSmartRouting() {
		t.Fatal("SmartAccess=false should disable smart routing even with empty model limits")
	}
}

func TestSmartAccessPersisted(t *testing.T) {
	newTestDB(t)
	user := &User{Username: "smart-test", Password: "x", Role: 1, Status: 1, Quota: 0, Group: "default", AffCode: common.GenerateAffCode()}
	CreateUser(user)

	// Create with SmartAccess=false
	f := false
	tok := &Token{
		UserID:      user.ID,
		Name:        "smart-off",
		Status:      common.StatusEnabled,
		ExpiredTime: -1,
		RemainQuota: 100,
		SmartAccess: &f,
	}
	if err := InsertToken(tok); err != nil {
		t.Fatal(err)
	}

	// Read back and verify.
	got, err := GetTokenByID(tok.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.SmartAccess == nil || *got.SmartAccess {
		t.Fatal("SmartAccess should be persisted as false")
	}
	if got.AllowsSmartRouting() {
		t.Fatal("persisted token with SmartAccess=false should not allow smart routing")
	}
}

// Ensure common is used.
var _ = common.StatusEnabled

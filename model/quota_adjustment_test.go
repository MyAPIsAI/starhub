package model

import (
	"testing"

	"starhub/common"
)

func TestAdjustUserQuotaCredit(t *testing.T) {
	newTestDB(t)
	user := &User{Username: "adjust-test", Password: "x", Role: 1, Status: 1, Quota: 1000, Group: "default", AffCode: common.GenerateAffCode()}
	CreateUser(user)

	entry, err := AdjustUserQuota(user.ID, 500, 1000, 1, "manual credit by admin")
	if err != nil {
		t.Fatalf("AdjustUserQuota failed: %v", err)
	}
	if entry.Amount != 500 || entry.BalanceAfter != 1500 {
		t.Fatalf("unexpected entry: %+v", entry)
	}

	updated, _ := GetUserByID(user.ID)
	if updated.Quota != 1500 {
		t.Fatalf("quota = %d, want 1500", updated.Quota)
	}
}

func TestAdjustUserQuotaDebit(t *testing.T) {
	newTestDB(t)
	user := &User{Username: "debit-test", Password: "x", Role: 1, Status: 1, Quota: 1000, Group: "default", AffCode: common.GenerateAffCode()}
	CreateUser(user)

	entry, err := AdjustUserQuota(user.ID, -300, 1000, 1, "manual debit by admin")
	if err != nil {
		t.Fatalf("AdjustUserQuota failed: %v", err)
	}
	if entry.Amount != -300 || entry.BalanceAfter != 700 {
		t.Fatalf("unexpected entry: %+v", entry)
	}
}

func TestAdjustUserQuotaConflict(t *testing.T) {
	newTestDB(t)
	user := &User{Username: "conflict-test", Password: "x", Role: 1, Status: 1, Quota: 1000, Group: "default", AffCode: common.GenerateAffCode()}
	CreateUser(user)

	// Wrong expected quota.
	_, err := AdjustUserQuota(user.ID, 100, 999, 1, "should fail")
	if err != ErrAdjustmentConflict {
		t.Fatalf("expected ErrAdjustmentConflict, got %v", err)
	}

	// Quota unchanged.
	updated, _ := GetUserByID(user.ID)
	if updated.Quota != 1000 {
		t.Fatalf("quota should be unchanged after conflict, got %d", updated.Quota)
	}
}

func TestAdjustUserQuotaNegativeResult(t *testing.T) {
	newTestDB(t)
	user := &User{Username: "neg-test", Password: "x", Role: 1, Status: 1, Quota: 100, Group: "default", AffCode: common.GenerateAffCode()}
	CreateUser(user)

	_, err := AdjustUserQuota(user.ID, -200, 100, 1, "over-debit")
	if err == nil {
		t.Fatal("should reject adjustment resulting in negative quota")
	}

	// Quota unchanged.
	updated, _ := GetUserByID(user.ID)
	if updated.Quota != 100 {
		t.Fatalf("quota should be unchanged, got %d", updated.Quota)
	}
}

func TestAdjustUserQuotaDisabledUser(t *testing.T) {
	newTestDB(t)
	user := &User{Username: "disabled-test", Password: "x", Role: 1, Status: 2, Quota: 1000, Group: "default", AffCode: common.GenerateAffCode()}
	CreateUser(user)

	_, err := AdjustUserQuota(user.ID, 100, 1000, 1, "disabled user")
	if err == nil {
		t.Fatal("should reject adjustment for disabled user")
	}
}

func TestAdjustUserQuotaZeroAmount(t *testing.T) {
	newTestDB(t)
	user := &User{Username: "zero-test", Password: "x", Role: 1, Status: 1, Quota: 1000, Group: "default", AffCode: common.GenerateAffCode()}
	CreateUser(user)

	_, err := AdjustUserQuota(user.ID, 0, 1000, 1, "zero")
	if err == nil {
		t.Fatal("should reject zero amount")
	}
}

func TestAdjustUserQuotaEmptyReason(t *testing.T) {
	newTestDB(t)
	user := &User{Username: "noreason-test", Password: "x", Role: 1, Status: 1, Quota: 1000, Group: "default", AffCode: common.GenerateAffCode()}
	CreateUser(user)

	_, err := AdjustUserQuota(user.ID, 100, 1000, 1, "")
	if err == nil {
		t.Fatal("should reject empty reason")
	}
}

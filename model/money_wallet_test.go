package model

import (
	"errors"
	"sync"
	"testing"

	"starhub/common"
)

func TestMoneyAdjustmentIsAuthorizedIdempotentAndAuditedAtNanoPrecision(t *testing.T) {
	newTestDB(t)
	u, _ := newBillingUser(t, 1000)
	root := &User{Username: "money-root", Role: common.RoleRoot, Status: common.StatusEnabled, AffCode: common.GenerateAffCode()}
	if err := CreateUser(root); err != nil {
		t.Fatal(err)
	}
	input := MoneyAdjustment{ActorID: root.ID, UserID: u.ID, OperationID: "one-grant", AmountNano: "1", ExpectedNano: "2000000", Reason: "Verified administrative grant"}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := AdjustMoneyBalance(input); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	account, err := ReadMoneyAccount(u.ID)
	if err != nil || account.AvailableNano != "2000001" || account.Sequence != 2 {
		t.Fatalf("grant duplicated or truncated: %+v %v", account, err)
	}
	var audits int64
	DB.Model(&AuditLog{}).Where("path = ?", "/api/wallet/adjustments").Count(&audits)
	if audits != 1 {
		t.Fatal("business operation audit duplicated")
	}
	input.AmountNano = "2"
	if _, err := AdjustMoneyBalance(input); !errors.Is(err, ErrMoneyConflict) {
		t.Fatal("conflicting grant replay accepted")
	}
	input.OperationID = "forbidden"
	input.ActorID = u.ID
	input.ExpectedNano = "2000001"
	if _, err := AdjustMoneyBalance(input); !errors.Is(err, ErrMoneyAuthorization) {
		t.Fatal("unprivileged grant accepted")
	}
	if err := VerifyMoneyLedger(u.ID); err != nil {
		t.Fatal(err)
	}
}

func TestMoneyAdjustmentAuditFailureRollsBackAllFundsAndLedger(t *testing.T) {
	newTestDB(t)
	u, _ := newBillingUser(t, 0)
	root := &User{Username: "audit-root", Role: common.RoleRoot, Status: common.StatusEnabled, AffCode: common.GenerateAffCode()}
	if err := CreateUser(root); err != nil {
		t.Fatal(err)
	}
	if err := DB.Exec(`CREATE TRIGGER fail_money_audit BEFORE INSERT ON audit_logs BEGIN SELECT RAISE(ABORT,'injected'); END`).Error; err != nil {
		t.Fatal(err)
	}
	input := MoneyAdjustment{ActorID: root.ID, UserID: u.ID, OperationID: "audit-failure", AmountNano: "1", ExpectedNano: "0", Reason: "Audited grant"}
	if _, err := AdjustMoneyBalance(input); err == nil {
		t.Fatal("audit failure did not abort funding")
	}
	got, _ := GetUserByID(u.ID)
	if got.Quota != 0 || got.QuotaRemainderNano != 0 {
		t.Fatal("failed audit left spendable money")
	}
	var entries int64
	DB.Model(&MoneyEntry{}).Where("user_id = ?", u.ID).Count(&entries)
	if entries != 0 {
		t.Fatal("failed audit left journal entries")
	}
	DB.Exec(`DROP TRIGGER fail_money_audit`)
	if _, err := AdjustMoneyBalance(input); err != nil {
		t.Fatal(err)
	}
	input.OperationID = "stale"
	if _, err := AdjustMoneyBalance(input); !errors.Is(err, ErrMoneyConflict) {
		t.Fatal("stale exact balance accepted")
	}
	if err := VerifyMoneyLedger(u.ID); err != nil {
		t.Fatal(err)
	}
}

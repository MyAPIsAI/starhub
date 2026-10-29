package model

import (
	"math"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"starhub/common"
)

func TestDeactivationPreviewUsesExactBalancesAndDoesNotWrite(t *testing.T) {
	newTestDB(t)
	owner := &User{Username: "preview-owner", Password: "synthetic-hash", Email: "private@example.test", Status: 1, Role: 1, Group: "default", Quota: 123456}
	if err := CreateUser(owner); err != nil {
		t.Fatal(err)
	}
	initial, err := ReadAccountDeactivationState(owner.ID)
	if err != nil || initial.Balance != 0 || len(initial.BlockedReasons) != 0 {
		t.Fatal("missing wallet was not a real zero initial balance")
	}
	var wallets int64
	DB.Model(&Wallet{}).Count(&wallets)
	if wallets != 0 {
		t.Fatal("readonly preview created a wallet")
	}
	if err := DB.Create(&Wallet{UserID: owner.ID}).Error; err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		amount int64
		reason string
		ack    bool
	}{{-1, "outstanding_balance", false}, {0, "", false}, {1, "", true}, {9_999_999, "", true}, {10_000_000, "balance_requires_resolution", false}, {math.MaxInt64, "balance_requires_resolution", false}} {
		if err := DB.Model(&Wallet{}).Where("user_id = ?", owner.ID).Update("balance", test.amount).Error; err != nil {
			t.Fatal(err)
		}
		var before Wallet
		DB.First(&before, "user_id = ?", owner.ID)
		state, err := ReadAccountDeactivationState(owner.ID)
		if err != nil || state.Balance != test.amount || state.RequiresDustAcknowledgement != test.ack || len(state.AccountRevision) != 64 {
			t.Fatalf("wrong exact preview for %d: %+v %v", test.amount, state, err)
		}
		if test.reason == "" && len(state.BlockedReasons) != 0 || test.reason != "" && !reflect.DeepEqual(state.BlockedReasons, []string{test.reason}) {
			t.Fatalf("wrong balance blocker: %+v", state)
		}
		var after Wallet
		DB.First(&after, "user_id = ?", owner.ID)
		updated, _ := GetUserByID(owner.ID)
		if before != after || *updated != *owner {
			t.Fatal("preview changed wallet/user metadata")
		}
	}
}

func TestDeactivationPreviewFreezesSecurityAndUnfinishedRechargeState(t *testing.T) {
	newTestDB(t)
	owner := &User{Username: "preview-freeze", Password: "synthetic-hash", Status: 1, Role: 1, Group: "default"}
	if err := CreateUser(owner); err != nil {
		t.Fatal(err)
	}
	before, _ := ReadAccountDeactivationState(owner.ID)
	if err := DB.Model(&User{}).Where("id = ?", owner.ID).Update("email", "changed@example.test").Error; err != nil {
		t.Fatal(err)
	}
	after, _ := ReadAccountDeactivationState(owner.ID)
	if before.AccountRevision == after.AccountRevision {
		t.Fatal("independent profile change retained a stale close revision")
	}
	for _, status := range []string{RechargePending, RechargeProcessing, RechargePaid, RechargeFailed, RechargeCancelled, RechargeExpired} {
		if err := DB.Create(&RechargeOrder{UserID: owner.ID, OrderNumber: "preview-" + status, Status: status, ExpiresAt: 1}).Error; err != nil {
			t.Fatal(err)
		}
	}
	state, err := ReadAccountDeactivationState(owner.ID)
	if err != nil || state.PendingRecharges != 2 || !reflect.DeepEqual(state.BlockedReasons, []string{"pending_recharges"}) || state.AccountRevision == after.AccountRevision {
		t.Fatal("unfinished persisted orders were not guarded")
	}
	var pending int64
	DB.Model(&RechargeOrder{}).Where("user_id = ? AND status IN ?", owner.ID, []string{RechargePending, RechargeProcessing}).Count(&pending)
	if pending != 2 {
		t.Fatal("preview silently expired or resolved orders")
	}
	if err := DB.Model(&User{}).Where("id = ?", owner.ID).Update("status", common.StatusDisabled).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := ReadAccountDeactivationState(owner.ID); err != ErrDeactivationUserUnavailable {
		t.Fatal("disabled user received a usable preview")
	}
}

func TestDeactivationPreviewIdentifiesTheLastActiveRoot(t *testing.T) {
	newTestDB(t)
	root := &User{Username: "preview-root", Role: 100, Status: 1, Group: "default", AffCode: "preview-root-a"}
	if err := CreateUser(root); err != nil {
		t.Fatal(err)
	}
	state, err := ReadAccountDeactivationState(root.ID)
	if err != nil || !state.LastActiveRoot || !reflect.DeepEqual(state.BlockedReasons, []string{"last_active_root"}) {
		t.Fatal("last active Root not identified")
	}
	other := &User{Username: "preview-another-root", Role: 100, Status: 1, Group: "default", AffCode: "preview-root-b"}
	if err := CreateUser(other); err != nil {
		t.Fatal(err)
	}
	next, err := ReadAccountDeactivationState(root.ID)
	if err != nil || next.LastActiveRoot || len(next.BlockedReasons) != 0 || state.AccountRevision == next.AccountRevision {
		t.Fatal("active Root change did not update preview")
	}
}

func TestDeactivationPreviewUsesOneSQLiteReadSnapshot(t *testing.T) {
	newTestDB(t)
	owner := &User{Username: "preview-snapshot", Password: "synthetic-hash", Role: 1, Status: 1, Group: "default"}
	if err := CreateUser(owner); err != nil {
		t.Fatal(err)
	}
	if err := DB.Create(&Wallet{UserID: owner.ID, Balance: 1}).Error; err != nil {
		t.Fatal(err)
	}
	expected, _ := ReadAccountDeactivationState(owner.ID)
	entered, release := make(chan struct{}), make(chan struct{})
	var once atomic.Bool
	name := "deactivation-preview-snapshot-barrier"
	if err := DB.Callback().Query().After("gorm:query").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "users" && once.CompareAndSwap(false, true) {
			close(entered)
			<-release
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = DB.Callback().Query().Remove(name) })
	type result struct {
		state *AccountDeactivationState
		err   error
	}
	done := make(chan result, 1)
	go func() { state, err := ReadAccountDeactivationState(owner.ID); done <- result{state, err} }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("preview did not enter its read transaction")
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&User{}).Where("id = ?", owner.ID).Update("email", "new@example.test").Error; err != nil {
			return err
		}
		return tx.Model(&Wallet{}).Where("user_id = ?", owner.ID).Update("balance", 2).Error
	})
	close(release)
	got := <-done
	if err != nil || got.err != nil || !reflect.DeepEqual(expected, got.state) {
		t.Fatalf("preview mixed read snapshots: %+v %v %v", got.state, got.err, err)
	}
	_ = DB.Callback().Query().Remove(name)
	latest, err := ReadAccountDeactivationState(owner.ID)
	if err != nil || latest.Balance != 2 || latest.AccountRevision == expected.AccountRevision {
		t.Fatal("fresh preview did not see committed changes")
	}
}

package model

import (
	"math"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestSimulatedRechargeCannotCertifyItsFundingSourceOrCreditAPIQuota(t *testing.T) {
	newTestDB(t)
	user := &User{Username: "simulation-owner", Quota: 123456}
	if err := CreateUser(user); err != nil {
		t.Fatal(err)
	}
	if _, err := GetOrCreateWallet(user.ID); err != nil {
		t.Fatal(err)
	}
	order := &RechargeOrder{UserID: user.ID, Amount: 10_000_000_000, FundingSource: "verified_provider"}
	if err := CreateRechargeOrder(order); err != nil || order.FundingSource != FundingSourceSimulation {
		t.Fatalf("create accepted invented source: %+v %v", order, err)
	}
	if err := DB.Model(order).Update("funding_source", "verified_provider").Error; err != nil {
		t.Fatal(err)
	}
	paid, err := ResolveRechargeOrder(order.ID, "success")
	if err != nil || paid.FundingSource != FundingSourceSimulation || paid.Status != RechargePaid {
		t.Fatalf("user result certified funds: %+v %v", paid, err)
	}
	entries, err := ListLedger(user.ID)
	if err != nil || len(entries) != 1 || entries[0].FundingSource != FundingSourceSimulation || entries[0].Amount != order.Amount {
		t.Fatalf("ledger source: %+v %v", entries, err)
	}
	before := entries[0]
	for _, outcome := range []string{"success", "failed", "cancelled"} {
		if again, err := ResolveRechargeOrder(order.ID, outcome); err != nil || !reflect.DeepEqual(paid, again) {
			t.Fatalf("terminal source changed: %+v %v", again, err)
		}
	}
	entries, _ = ListLedger(user.ID)
	if len(entries) != 1 || !reflect.DeepEqual(entries[0], before) {
		t.Fatal("repeat resolution rewrote ledger")
	}
	owner, _ := GetUserByID(user.ID)
	wallet, _ := GetOrCreateWallet(user.ID)
	if owner.Quota != 123456 || wallet.Balance != order.Amount {
		t.Fatal("simulated payment changed API quota or duplicated wallet credit")
	}
	for _, source := range []string{"", "verified_provider", "trusted", "SIMULATION", FundingSourceLegacyUnverified} {
		if PublicFundingSource(source) != FundingSourceLegacyUnverified {
			t.Fatalf("unverified source promoted: %q", source)
		}
	}
}

func TestFundingSourceMigrationPreservesLegacyMoneyAndHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-funding.db")
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	user := &User{Username: "legacy-owner", Quota: 123456}
	if err := CreateUser(user); err != nil {
		t.Fatal(err)
	}
	wallet := Wallet{UserID: user.ID, Balance: math.MaxInt64}
	if err := DB.Create(&wallet).Error; err != nil {
		t.Fatal(err)
	}
	old := RechargeOrder{UserID: user.ID, OrderNumber: "legacy-paid", Amount: math.MaxInt64, Status: RechargePaid, PaidAt: 1700000000, CreatedAt: 1699999990, ExpiresAt: 1700001000}
	if err := DB.Create(&old).Error; err != nil {
		t.Fatal(err)
	}
	entry := LedgerEntry{UserID: user.ID, Type: "recharge", Amount: math.MaxInt64, BalanceAfter: math.MaxInt64, OrderID: old.OrderNumber, TraceID: "legacy-trace", Description: "Historical unverified credit", CreatedAt: 1700000000}
	if err := DB.Create(&entry).Error; err != nil {
		t.Fatal(err)
	}
	for _, table := range []any{&RechargeOrder{}, &LedgerEntry{}} {
		if err := DB.Migrator().DropColumn(table, "FundingSource"); err != nil {
			t.Fatal(err)
		}
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	got, err := GetRechargeOrder(old.ID)
	if err != nil || got.FundingSource != FundingSourceLegacyUnverified {
		t.Fatalf("legacy order promoted: %+v %v", got, err)
	}
	old.FundingSource = FundingSourceLegacyUnverified
	if !reflect.DeepEqual(*got, old) {
		t.Fatal("migration rewrote historical order")
	}
	entries, err := ListLedger(user.ID)
	entry.FundingSource = FundingSourceLegacyUnverified
	if err != nil || len(entries) != 1 || !reflect.DeepEqual(entries[0], entry) {
		t.Fatal("migration rewrote historical ledger")
	}
	again, err := ResolveRechargeOrder(old.ID, "success")
	if err != nil || !reflect.DeepEqual(again, got) {
		t.Fatal("reading paid legacy order recertified history")
	}
	stored, _ := GetOrCreateWallet(user.ID)
	owner, _ := GetUserByID(user.ID)
	if !reflect.DeepEqual(*stored, wallet) || owner.Quota != user.Quota {
		t.Fatal("migration changed balances or API quota")
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	reopened, _ := GetRechargeOrder(old.ID)
	if !reflect.DeepEqual(reopened, got) {
		t.Fatal("provenance not durable")
	}
}

func TestLegacyPendingManualConfirmationIsExplicitlySimulatedAndRollsBackWithLedger(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "ledger-failure"}[fail], func(t *testing.T) {
			newTestDB(t)
			user := &User{Username: "legacy-pending", Quota: 123}
			if err := CreateUser(user); err != nil {
				t.Fatal(err)
			}
			wallet, err := GetOrCreateWallet(user.ID)
			if err != nil {
				t.Fatal(err)
			}
			order := RechargeOrder{UserID: user.ID, OrderNumber: "old-pending", Amount: 10_000_000_000, Status: RechargePending, ExpiresAt: time.Now().Unix() + 3600}
			if err := DB.Create(&order).Error; err != nil {
				t.Fatal(err)
			}
			if fail {
				if err := DB.Exec(`CREATE TRIGGER fail_simulated_source BEFORE INSERT ON ledger_entries BEGIN SELECT RAISE(ABORT,'injected'); END`).Error; err != nil {
					t.Fatal(err)
				}
			}
			_, err = ResolveRechargeOrder(order.ID, "success")
			stored, _ := GetRechargeOrder(order.ID)
			balance, _ := GetOrCreateWallet(user.ID)
			owner, _ := GetUserByID(user.ID)
			entries, _ := ListLedger(user.ID)
			if fail {
				if err == nil || !reflect.DeepEqual(stored, &order) || !reflect.DeepEqual(balance, wallet) || len(entries) != 0 {
					t.Fatal("failed resolution left partial source or money")
				}
			} else if err != nil || stored.FundingSource != FundingSourceSimulation || len(entries) != 1 || entries[0].FundingSource != FundingSourceSimulation {
				t.Fatal("manual resolution not identified as simulated")
			}
			if owner.Quota != 123 {
				t.Fatal("manual confirmation minted API quota")
			}
		})
	}
}

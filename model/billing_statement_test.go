package model

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestAPIBillingOutboxRollsBackWithSettlement(t *testing.T) {
	newTestDB(t)
	user, key := newBillingUser(t, 1000)
	if err := ReserveQuota(user.ID, key.ID, false, 100); err != nil {
		t.Fatal(err)
	}
	if err := DB.Exec(`CREATE TRIGGER reject_billing_event BEFORE INSERT ON api_charge_events BEGIN SELECT RAISE(ABORT,'synthetic outbox failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if err := SettleQuota(user.ID, key.ID, false, -50, 50, &Log{}); err == nil {
		t.Fatal("settlement committed without its durable billing event")
	}
	fresh, _ := GetUserByID(user.ID)
	token, _ := GetTokenByID(key.ID)
	count, _ := CountLogs(user.ID)
	if fresh.Quota != 900 || fresh.UsedQuota != 0 || token.UsedQuota != 0 || count != 0 {
		t.Fatal("failed journal write partially settled usage")
	}
}

func TestAPIBillingJournalDoesNotMoveSimulationFunds(t *testing.T) {
	newTestDB(t)
	user, key := newBillingUser(t, 1000)
	wallet, err := GetOrCreateWallet(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := DB.Model(wallet).UpdateColumn("balance", int64(10000000000)).Error; err != nil {
		t.Fatal(err)
	}
	if err := ReserveQuota(user.ID, key.ID, false, 20); err != nil {
		t.Fatal(err)
	}
	if err := SettleQuota(user.ID, key.ID, false, 0, 20, &Log{Status: 1}); err != nil {
		t.Fatal(err)
	}
	if err := FlushAPICharges(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	wallet, err = GetOrCreateWallet(user.ID)
	if err != nil || wallet.Balance != 10000000000 {
		t.Fatal("API ledger moved simulated wallet money")
	}
	fresh, _ := GetUserByID(user.ID)
	if fresh.Quota != 980 {
		t.Fatal("journal added or debited API funds again")
	}
	summary, err := ReadFinanceSummary(context.Background(), time.Now().Add(-time.Hour).Unix(), time.Now().Add(time.Hour).Unix())
	if err != nil || summary.WalletAndAPIQuotaLinked || summary.APICollectedNano != "40000" || summary.LedgerDebitsNano != "0" {
		t.Fatalf("independent finance totals mixed currencies: %+v %v", summary, err)
	}
}

func TestAPIBillingConcurrentFlushIsIdempotentAndStatementGroupsByKey(t *testing.T) {
	newTestDB(t)
	user, key := newBillingUser(t, 1000)
	for i := 0; i < 3; i++ {
		if err := ReserveQuota(user.ID, key.ID, false, 10); err != nil {
			t.Fatal(err)
		}
		if err := SettleQuota(user.ID, key.ID, false, 0, 10, &Log{TraceID: "synthetic", Status: 1}); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	errors := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errors <- FlushAPICharges(context.Background(), true) }()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	raw, err := ListLedger(user.ID)
	if err != nil || len(raw) != 1 || raw[0].Amount != -60000 || raw[0].RequestCount != 3 {
		t.Fatalf("incorrect minute batch: %+v %v", raw, err)
	}
	// Flushing later usage appends another immutable batch in the same minute.
	original := raw[0]
	if err := ReserveQuota(user.ID, key.ID, false, 10); err != nil {
		t.Fatal(err)
	}
	if err := SettleQuota(user.ID, key.ID, false, 0, 10, &Log{Status: 1}); err != nil {
		t.Fatal(err)
	}
	statement, err := StatementLedger(user.ID, 0, 0, "")
	if err != nil || len(statement) != 1 || statement[0].Amount != -80000 || statement[0].RequestCount != 4 {
		t.Fatalf("incorrect daily statement: %+v %v", statement, err)
	}
	var unchanged LedgerEntry
	if err := DB.First(&unchanged, original.ID).Error; err != nil {
		t.Fatal(err)
	}
	if unchanged.Amount != original.Amount || unchanged.BalanceAfter != original.BalanceAfter {
		t.Fatal("flushing modified an existing ledger entry")
	}
	fresh, _ := GetUserByID(user.ID)
	if fresh.Quota != 960 || fresh.UsedQuota != 40 {
		t.Fatal("ledger flush debited account a second time")
	}
	rows, err := StatementLedger(user.ID, time.Now().AddDate(0, 0, -1).Unix(), time.Now().AddDate(0, 0, 1).Unix(), "synthetic")
	if err != nil || len(rows) != 1 || rows[0].TraceID != "synthetic" {
		t.Fatal("trace lookup did not resolve the billed batch")
	}
}

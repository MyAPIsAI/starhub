package model

import (
	"math"
	"sync"
	"sync/atomic"
	"testing"
)

func TestLedgerMoneyDistinguishesHistoricalAccountsWithoutRewriting(t *testing.T) {
	for _, sample := range []struct {
		entry           LedgerEntry
		account, amount string
	}{
		{LedgerEntry{Type: "adjustment", Description: "by admin 1: credit", Amount: 500}, LedgerAccountQuota, "1000000"},
		{LedgerEntry{Type: "adjustment", Description: "Account closure dust write-off", Amount: -500}, LedgerAccountWallet, "-500"},
		{LedgerEntry{Type: "recharge", Amount: math.MaxInt64}, LedgerAccountWallet, "9223372036854775807"},
		{LedgerEntry{Account: LedgerAccountQuota, AmountUnit: LedgerUnitLegacyQuota, Amount: math.MinInt64}, LedgerAccountQuota, "-18446744073709551616000"},
	} {
		before := sample.entry
		account, _, amount, _, err := LedgerMoney(sample.entry)
		if err != nil || account != sample.account || amount != sample.amount || before != sample.entry {
			t.Fatalf("monetary identity: %s %s %v", account, amount, err)
		}
	}
	if _, _, _, _, err := LedgerMoney(LedgerEntry{Account: LedgerAccountWallet, AmountUnit: LedgerUnitLegacyQuota}); err == nil {
		t.Fatal("wallet quota identity accepted")
	}
}

func TestQuotaAdjustmentConcurrentRevisionAndOverflow(t *testing.T) {
	newTestDB(t)
	u, _ := newBillingUser(t, 1000)
	var wg sync.WaitGroup
	var successes atomic.Int32
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := AdjustUserQuota(u.ID, 500, 1000, 1, "concurrent revision")
			if err == nil {
				successes.Add(1)
			} else if err != ErrAdjustmentConflict {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	var count int64
	DB.Model(&LedgerEntry{}).Where("user_id = ?", u.ID).Count(&count)
	if successes.Load() != 1 || count != 1 {
		t.Fatalf("adjustments=%d ledger=%d", successes.Load(), count)
	}
	if err := DB.Model(u).Update("quota", math.MaxInt64).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := AdjustUserQuota(u.ID, 1, math.MaxInt64, 1, "overflow"); err == nil {
		t.Fatal("overflow accepted")
	}
	got, _ := GetUserByID(u.ID)
	if got.Quota != math.MaxInt64 {
		t.Fatal("overflow changed quota")
	}
}

package model

import (
	"context"
	"math"
	"testing"
)

func TestFinanceSummaryKeepsExactIndependentLedgersAndWindow(t *testing.T) {
	newTestDB(t)
	for _, wallet := range []Wallet{{UserID: 1, Balance: math.MaxInt64}, {UserID: 2, Balance: math.MaxInt64}} {
		if err := DB.Create(&wallet).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, order := range []RechargeOrder{
		{OrderNumber: "paid-a", Amount: math.MaxInt64, Status: RechargePaid, PaidAt: 2000},
		{OrderNumber: "paid-b", Amount: math.MaxInt64, Status: RechargePaid, PaidAt: 2999},
		{OrderNumber: "pending", Amount: 400, Status: RechargePending, ExpiresAt: 1500, PaidAt: 2000},
		{OrderNumber: "unknown-time", Amount: 500, Status: RechargePaid, PaidAt: 0},
		{OrderNumber: "outside", Amount: 600, Status: RechargePaid, PaidAt: 3000},
	} {
		if err := DB.Create(&order).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, entry := range []LedgerEntry{{Amount: math.MaxInt64, CreatedAt: 2000}, {Amount: math.MinInt64, CreatedAt: 2999}, {Amount: 99, CreatedAt: 3000}} {
		if err := DB.Create(&entry).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, entry := range []Log{{Quota: math.MaxInt64, UnfundedQuota: 3, Status: 1, CreatedAt: 2000}, {Quota: 1, UnfundedQuota: 7, Status: 3, CreatedAt: 2999}, {Quota: 0, Status: 2, CreatedAt: 2500}, {Quota: 99, Status: 1, CreatedAt: 1999}, {Quota: 99, Status: 1, CreatedAt: 3000}} {
		if err := DB.Create(&entry).Error; err != nil {
			t.Fatal(err)
		}
	}
	summary, err := ReadFinanceSummary(context.Background(), 2000, 3000)
	if err != nil {
		t.Fatal(err)
	}
	if summary.WalletBalanceNano != "18446744073709551614" || summary.PaidRechargeNano != "18446744073709551614" || summary.PaidRechargeCount != 2 || summary.PaidOrdersWithoutTimestamp != 1 {
		t.Fatalf("wallet/order arithmetic or scope changed: %+v", summary)
	}
	if summary.LedgerCreditsNano != "9223372036854775807" || summary.LedgerDebitsNano != "9223372036854775808" {
		t.Fatalf("signed ledger overflowed: %+v", summary)
	}
	if summary.APICollectedNano != "18446744073709551616000" || summary.APIUnfundedNano != "20000" || summary.APIRequestCount != 3 || summary.APISuccessCount != 1 || summary.APIFailedCount != 1 || summary.APICancelledCount != 1 || summary.WalletAndAPIQuotaLinked {
		t.Fatalf("API balances/statuses combined or overflowed: %+v", summary)
	}
	var pending RechargeOrder
	if err := DB.Where("order_number = ?", "pending").First(&pending).Error; err != nil || pending.Status != RechargePending {
		t.Fatalf("read mutated order state: %+v %v", pending, err)
	}
	var wallets int64
	if err := DB.Model(&Wallet{}).Count(&wallets).Error; err != nil || wallets != 2 {
		t.Fatal("read created wallets")
	}
}

func TestFinanceSummaryDoesNotReturnPartialOrInventedTotals(t *testing.T) {
	newTestDB(t)
	for _, window := range [][2]int64{{0, 100}, {100, 100}, {200, 100}} {
		if result, err := ReadFinanceSummary(context.Background(), window[0], window[1]); err == nil || result != nil {
			t.Fatalf("invalid window accepted: %+v %v", result, err)
		}
	}
	result, err := ReadFinanceSummary(context.Background(), 100, 200)
	if err != nil || result.WalletBalanceNano != "0" || result.PaidRechargeNano != "0" || result.APICollectedNano != "0" || result.LedgerDebitsNano != "0" {
		t.Fatalf("empty database did not yield exact zeros: %+v %v", result, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if result, err := ReadFinanceSummary(ctx, 100, 200); err == nil || result != nil {
		t.Fatal("canceled read returned a summary")
	}
	if err := DB.Migrator().DropTable(&LedgerEntry{}); err != nil {
		t.Fatal(err)
	}
	if result, err := ReadFinanceSummary(context.Background(), 100, 200); err == nil || result != nil {
		t.Fatal("broken data source returned partial zero totals")
	}
}

package model

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"starhub/common"
)

func TestADMAC07OperatingDashboardMatchesLedgerAndCalls(t *testing.T) {
	newTestDB(t)
	loc, _ := time.LoadLocation("Asia/Dubai")
	start := time.Date(2026, 11, 5, 10, 0, 0, 0, loc)
	end, now := start.Add(time.Hour), start.Add(66*time.Minute)
	user := &User{Username: "dashboard-customer", Role: 1, Status: 1, CreatedAt: start.Unix(), AffCode: "dashboard-customer", Quota: common.QuotaPerUSD * 1000}
	if err := CreateUser(user); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		row, err := StaffAdjustBalance(0, user.ID, 100_000_000_000, "offline_payment", "Synthetic receipt", fmt.Sprint(i))
		if err != nil {
			t.Fatal(err)
		}
		if err := DB.Model(row).Update("created_at", start.Add(time.Duration(i)*time.Minute).Unix()).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := DB.Transaction(func(tx *gorm.DB) error {
		for i := 0; i < 1000; i++ {
			row := &Log{UserID: user.ID, Model: fmt.Sprintf("model-%d", i%25), Vendor: fmt.Sprintf("vendor-%d", i%3), ServiceGroup: fmt.Sprintf("group-%d", i%4), CallType: "chat", Quota: 12500, Status: 1, CreatedAt: start.Add(time.Duration(i) * time.Second).Unix(), UsedChannels: "1,2", TraceID: fmt.Sprintf("synthetic-%d", i)}
			if err := tx.Create(row).Error; err != nil {
				return err
			}
			if err := queueAPICharge(tx, row); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := FlushAPICharges(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	read := func() *OperatingDashboard {
		t.Helper()
		report, err := ReadOperatingDashboard(context.Background(), start, end, now, "middle-east")
		if err != nil {
			t.Fatal(err)
		}
		return report
	}
	report := read()
	if report.Totals.RechargeNano != "300000000000" || report.Totals.ConsumptionNano != "25000000000" || report.Totals.Requests != 1000 || report.Totals.Successful != 1000 || report.Totals.ActiveAccounts != 1 || report.Totals.PayingAccounts != 1 || report.Totals.NewAccounts != 1 {
		t.Fatalf("AC07 totals: %+v", report.Totals)
	}
	if report.UpdatedAt < end.Unix() || report.RefreshSeconds != 300 || report.Timezone != "Asia/Dubai" {
		t.Fatal("invalid freshness or timezone")
	}
	debit, err := sumMoney(DB.Model(&LedgerEntry{}).Where("user_id = ? AND funding_source = ? AND created_at >= ? AND created_at < ?", user.ID, FundingSourceAPIQuota, start.Unix(), end.Unix()), "amount")
	if err != nil || debit.Neg(debit).String() != report.Totals.ConsumptionNano {
		t.Fatalf("ledger mismatch: %s %v", debit, err)
	}
	for _, rows := range [][]OperatingBucket{report.Groups, report.Vendors, report.CallTypes, report.Models, report.TopAccounts, report.Series[0].Groups} {
		total := new(big.Int)
		for _, row := range rows {
			amount, _ := new(big.Int).SetString(row.AmountNano, 10)
			total.Add(total, amount)
		}
		if total.String() != report.Totals.ConsumptionNano {
			t.Fatalf("distribution differs: %s", total)
		}
	}
	if len(report.Models) != 25 || len(report.TopModels) != 20 || len(report.Series) != 1 {
		t.Fatal("ranking or hourly series incorrect")
	}
	// Reads cannot create additional ledger rows or rebook funds.
	before, _ := json.Marshal(report)
	after, _ := json.Marshal(read())
	if string(before) != string(after) {
		t.Fatal("repeated reads changed the snapshot")
	}
	if err := DB.Create(&Log{UserID: user.ID, Model: "late-settlement", Status: 2, CreatedAt: start.Add(59 * time.Minute).Unix()}).Error; err != nil {
		t.Fatal(err)
	}
	if read().Totals.Requests != 1001 || read().Totals.ConsumptionNano != "25000000000" {
		t.Fatal("refresh missed newly persisted data")
	}
}

func TestOperatingDashboardScopesFundingBoundariesAndPrecision(t *testing.T) {
	newTestDB(t)
	loc, _ := time.LoadLocation("Asia/Shanghai")
	start := time.Date(2026, 11, 5, 0, 0, 0, 0, loc)
	end, now := start.AddDate(0, 0, 1), start.Add(12*time.Hour)
	users := []*User{{Username: "paying", Role: 1, Status: 1, AffCode: "dash-a", CreatedAt: start.Unix()}, {Username: "disabled-active", Role: 1, Status: 2, AffCode: "dash-b", CreatedAt: start.Add(-time.Second).Unix()}, {Username: "enabled-idle", Role: 1, Status: 1, AffCode: "dash-c", CreatedAt: start.Unix()}, {Username: "staff", Role: 100, Status: 1, AffCode: "dash-d", CreatedAt: start.Unix()}}
	for _, user := range users {
		if err := CreateUser(user); err != nil {
			t.Fatal(err)
		}
	}
	entries := []Log{
		{UserID: users[0].ID, Quota: 1, Status: 1, CreatedAt: start.Unix()},
		{UserID: users[1].ID, Quota: 0, Status: 2, CreatedAt: start.Add(time.Hour).Unix()},
		{UserID: users[1].ID, Quota: 0, Status: 3, CreatedAt: start.Add(time.Hour).Unix()},
		{UserID: users[3].ID, Quota: 999, Status: 1, CreatedAt: start.Unix()},
		{UserID: users[0].ID, Quota: 999, Status: 1, CreatedAt: start.Add(-time.Second).Unix()},
		{UserID: users[0].ID, Quota: 999, Status: 1, CreatedAt: end.Unix()},
		{UserID: users[0].ID, Quota: 999, Status: 1, CreatedAt: now.Add(time.Hour).Unix()},
	}
	if err := DB.Create(&entries).Error; err != nil {
		t.Fatal(err)
	}
	credits := []LedgerEntry{
		{UserID: users[0].ID, Type: "adjustment", FundingSource: FundingSourceStaffAdjustment, Description: "offline_payment · Receipt", Amount: math.MaxInt64, CreatedAt: start.Unix()},
		{UserID: users[0].ID, Type: "adjustment", FundingSource: FundingSourceStaffAdjustment, Description: "offline_payment · Receipt", Amount: math.MaxInt64, CreatedAt: start.Unix()},
		{UserID: users[0].ID, Type: "adjustment", FundingSource: FundingSourceStaffAdjustment, Description: "gift · offline_payment", Amount: 17, CreatedAt: start.Unix()},
		{UserID: users[1].ID, Type: "recharge", FundingSource: FundingSourceSimulation, Amount: 111, CreatedAt: start.Unix()},
		{UserID: users[1].ID, Type: "recharge", FundingSource: FundingSourceLegacyUnverified, Amount: 222, CreatedAt: start.Unix()},
		{UserID: users[3].ID, Type: "adjustment", FundingSource: FundingSourceStaffAdjustment, Description: "offline_payment · Staff", Amount: 999, CreatedAt: start.Unix()},
	}
	if err := DB.Create(&credits).Error; err != nil {
		t.Fatal(err)
	}
	result, err := ReadOperatingDashboard(context.Background(), start, end, now, "regional")
	if err != nil {
		t.Fatal(err)
	}
	if result.Totals.Requests != 3 || result.Totals.Successful != 1 || result.Totals.Cancelled != 1 || result.Totals.Failed != 1 || result.Totals.ActiveAccounts != 2 || result.Totals.NewAccounts != 2 || result.Totals.PayingAccounts != 1 {
		t.Fatalf("scope mismatch: %+v", result.Totals)
	}
	if result.Totals.ConsumptionNano != "2000" || result.Totals.RechargeNano != "18446744073709551614" {
		t.Fatalf("precision loss: %+v", result.Totals)
	}
	if result.Funding.SimulationNano != "111" || result.Funding.UnverifiedNano != "222" || result.Funding.OtherCreditNano != "17" {
		t.Fatalf("funding conflation: %+v", result.Funding)
	}
	if len(result.Groups) != 1 || result.Groups[0].ID != "" || len(result.Series) != 24 || result.Series[1].AmountNano != "0" {
		t.Fatal("empty history labels or buckets were invented")
	}
	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), "password") || strings.Contains(string(encoded), "trace_id") {
		t.Fatal("unexpected sensitive projection")
	}
}

func TestOperatingDashboardRegionalDSTAndEmptyState(t *testing.T) {
	newTestDB(t)
	loc, _ := time.LoadLocation("America/New_York")
	for _, test := range []struct {
		month      time.Month
		day, hours int
	}{{3, 8, 23}, {11, 1, 25}} {
		start := time.Date(2026, test.month, test.day, 0, 0, 0, 0, loc)
		end := start.AddDate(0, 0, 1)
		report, err := ReadOperatingDashboard(context.Background(), start, end, end, "dst")
		if err != nil || len(report.Series) != test.hours || report.Totals.ConsumptionNano != "0" || report.Groups == nil {
			t.Fatalf("DST/empty state: %+v %v", report, err)
		}
		for i := 1; i < len(report.Series); i++ {
			if report.Series[i].At-report.Series[i-1].At != 3600 {
				t.Fatal("repeated DST hour collapsed")
			}
		}
	}
	user := &User{Username: "dst-customer", Role: 1, Status: 1, AffCode: "dashboard-dst"}
	if err := CreateUser(user); err != nil {
		t.Fatal(err)
	}
	loc, _ = time.LoadLocation("Australia/Lord_Howe")
	for _, date := range []time.Time{time.Date(2026, 4, 5, 0, 0, 0, 0, loc), time.Date(2026, 10, 4, 0, 0, 0, 0, loc)} {
		end := date.AddDate(0, 0, 1)
		if err := DB.Create(&Log{UserID: user.ID, Quota: 1, Status: 1, CreatedAt: date.Add(6 * time.Hour).Unix()}).Error; err != nil {
			t.Fatal(err)
		}
		report, err := ReadOperatingDashboard(context.Background(), date, end, end, "half-hour-dst")
		if err != nil {
			t.Fatal(err)
		}
		total := new(big.Int)
		for _, point := range report.Series {
			amount, _ := new(big.Int).SetString(point.AmountNano, 10)
			total.Add(total, amount)
		}
		if report.Totals.ConsumptionNano != "2000" || total.String() != report.Totals.ConsumptionNano {
			t.Fatal("half-hour DST transition lost consumption from the hourly trend")
		}
	}
}

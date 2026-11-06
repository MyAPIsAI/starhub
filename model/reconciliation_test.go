package model

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"starhub/common"
	"starhub/config"
	"starhub/setting"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func reconciliationFixture(t *testing.T) (*User, *Token, time.Time) {
	t.Helper()
	newTestDB(t)
	old := config.RegionTimezone()
	_ = config.SetRegionTimezone("Asia/Dubai")
	t.Cleanup(func() { _ = config.SetRegionTimezone(old) })
	loc, _ := time.LoadLocation("Asia/Dubai")
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, loc)
	user := &User{Username: "reconciliation-customer", Role: 1, Status: 1, Quota: 100 * common.QuotaPerUSD, AffCode: "reconciliation"}
	if err := CreateUser(user); err != nil {
		t.Fatal(err)
	}
	if err := DB.Create(&LedgerEntry{UserID: user.ID, Type: "adjustment", Amount: 100_000_000_000, BalanceAfter: 100_000_000_000, FundingSource: FundingSourceStaffAdjustment, CreatedAt: now.AddDate(0, 0, -30).Unix()}).Error; err != nil {
		t.Fatal(err)
	}
	token := &Token{UserID: user.ID, Name: "Reconciliation key", UnlimitedQuota: true, Status: 1, Group: "default"}
	if err := InsertToken(token); err != nil {
		t.Fatal(err)
	}
	return user, token, now
}

func reconciliationReport(t *testing.T, u *User, day time.Time, now time.Time) ReconciliationReport {
	t.Helper()
	rows, err := RunDailyReconciliation(context.Background(), day.Format("2006-01-02"), u.ID, 0, now)
	if err != nil || len(rows) != 1 {
		t.Fatalf("report: %v %+v", err, rows)
	}
	return rows[0]
}
func containsDifference(r ReconciliationReport, kind string) bool {
	for _, d := range r.Differences {
		if d.Kind == kind {
			return true
		}
	}
	return false
}

func TestReconciliationBalanceAndReservations(t *testing.T) {
	u, key, now := reconciliationFixture(t)
	day := now.AddDate(0, 0, -1)
	if err := ReserveQuota(u.ID, key.ID, true, 2*common.QuotaPerUSD); err != nil {
		t.Fatal(err)
	}
	r := reconciliationReport(t, u, day, now)
	if len(r.Differences) != 0 || r.ReservedNano != "2000000000" {
		t.Fatalf("reservation falsely reported: %+v", r)
	}
	entry := &Log{AcceptedAt: day.Unix(), Status: 1}
	if err := SettleQuota(u.ID, key.ID, true, -common.QuotaPerUSD, common.QuotaPerUSD, entry); err != nil {
		t.Fatal(err)
	}
	r = reconciliationReport(t, u, day, now)
	if len(r.Differences) != 0 || r.BalanceActualNano != "99000000000" {
		t.Fatalf("settlement mismatch: %+v", r)
	}
	if err := ReserveQuota(u.ID, key.ID, true, 5*common.QuotaPerUSD); err != nil {
		t.Fatal(err)
	}
	if err := ReleaseQuota(u.ID, key.ID, true, 5*common.QuotaPerUSD); err != nil {
		t.Fatal(err)
	}
	if r := reconciliationReport(t, u, day, now); r.ReservedNano != "0" || len(r.Differences) != 0 {
		t.Fatalf("cancelled reservation: %+v", r)
	}
	DB.Model(&User{}).Where("id = ?", u.ID).Update("quota_nano_remainder", 1)
	r = reconciliationReport(t, u, day, now)
	if !containsDifference(r, "balance_vs_ledger") {
		t.Fatal("one nano balance drift was missed")
	}
	for _, d := range r.Differences {
		if d.Kind == "balance_vs_ledger" && d.Delta != "1" {
			t.Fatal(d)
		}
	}
}

func TestReconciliationWalletAndKeyDrift(t *testing.T) {
	u, key, now := reconciliationFixture(t)
	day := now.AddDate(0, 0, -1)
	DB.Create(&Wallet{UserID: u.ID, Balance: 10_000_000_000})
	DB.Create(&LedgerEntry{UserID: u.ID, Type: "recharge", FundingSource: FundingSourceSimulation, Amount: 10_000_000_000, CreatedAt: day.Unix()})
	DB.Create(&RechargeOrder{OrderNumber: "sim-review", UserID: u.ID, Amount: 10_000_000_000, Status: RechargePaid, FundingSource: FundingSourceSimulation, PaidAt: day.Unix()})
	entry := &Log{AcceptedAt: day.Unix()}
	if err := SettleQuota(u.ID, key.ID, true, common.QuotaPerUSD, common.QuotaPerUSD, entry); err != nil {
		t.Fatal(err)
	}
	r := reconciliationReport(t, u, day, now)
	if len(r.Differences) != 0 {
		t.Fatalf("independent wallet false positive: %+v", r.Differences)
	}
	DB.Model(&Wallet{}).Where("user_id = ?", u.ID).Update("balance", 9_000_000_000)
	DB.Model(&Token{}).Where("id = ?", key.ID).Update("used_quota", 2*common.QuotaPerUSD)
	r = reconciliationReport(t, u, day, now)
	if !containsDifference(r, "wallet_vs_ledger") || !containsDifference(r, "key_counter_vs_details") {
		t.Fatal(r.Differences)
	}
	DB.Model(&APIChargeEvent{}).Where("log_id = ?", entry.ID).Update("amount", 2_000_000_000)
	r = reconciliationReport(t, u, day, now)
	if !containsDifference(r, "key_usage_vs_details") {
		t.Fatal("daily key drift missed")
	}
}

func TestReconciliationAdmissionDayAndDurableRetry(t *testing.T) {
	u, key, now := reconciliationFixture(t)
	accepted := time.Date(2026, 11, 3, 23, 59, 50, 0, now.Location())
	finished := accepted.Add(30 * time.Second)
	entry := &Log{AcceptedAt: accepted.Unix(), SettledAt: finished.Unix(), DurationMs: 30000, TraceID: "midnight"}
	intent, err := RecordQuotaSettlement(u.ID, key.ID, true, common.QuotaPerUSD, common.QuotaPerUSD, entry)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = ApplyQuotaSettlement(intent.ID); err != nil {
		t.Fatal(err)
	}
	if _, claimed, err := ApplyQuotaSettlement(intent.ID); err != nil || claimed {
		t.Fatalf("duplicate charge: %v %v", claimed, err)
	}
	a, b := reconciliationReport(t, u, accepted, now), reconciliationReport(t, u, finished, now)
	if a.UsageNano != "1000000000" || b.UsageNano != "0" || len(a.Differences) != 0 || len(b.Differences) != 0 {
		t.Fatalf("wrong regional day: %+v %+v", a, b)
	}
}

func TestReconciliationSchedulerAndHistoricalBackfill(t *testing.T) {
	u, key, now := reconciliationFixture(t)
	ctx := context.Background()
	old := now.AddDate(0, 0, -10)
	reconciliationReport(t, u, old, now)
	DB.Create(&ReconciliationSchedule{ID: 1, Timezone: now.Location().String(), CompletedDay: "2026-11-03"})
	before := time.Date(2026, 11, 5, 1, 59, 59, 0, now.Location())
	if err := RunScheduledReconciliation(ctx, before); err != nil {
		t.Fatal(err)
	}
	var n int64
	DB.Model(&ReconciliationReport{}).Where("date = ?", "2026-11-04").Count(&n)
	if n != 0 {
		t.Fatal("ran before 02:00")
	}
	if err := SettleQuota(u.ID, key.ID, true, common.QuotaPerUSD, common.QuotaPerUSD, &Log{AcceptedAt: old.Unix()}); err != nil {
		t.Fatal(err)
	}
	if err := RunScheduledReconciliation(ctx, before.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	rows, err := ReadReconciliation(ctx, old.Format("2006-01-02"), 1, u.ID)
	if err != nil || len(rows) != 1 || rows[0].UsageNano != "1000000000" {
		t.Fatalf("historical backfill: %+v %v", rows, err)
	}
	DB.Model(&ReconciliationReport{}).Where("date = ?", "2026-11-04").Count(&n)
	if n != 1 {
		t.Fatal("02:00 report missing")
	}
	if err := RunScheduledReconciliation(ctx, now.AddDate(0, 0, 3)); err != nil {
		t.Fatal(err)
	}
	var state ReconciliationSchedule
	DB.First(&state, 1)
	if state.CompletedDay != "2026-11-07" {
		t.Fatal("missed restart catch-up")
	}
}

func TestReconciliationAlertsRetryAndDeduplicate(t *testing.T) {
	u, _, now := reconciliationFixture(t)
	ctx := context.Background()
	day := now.AddDate(0, 0, -1)
	old := setting.AlertWebhookURLValue()
	t.Cleanup(func() { _ = setting.UpdateAlert(setting.OptionKeyAlertWebhookURL, old) })
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Idempotency-Key") == "" {
			t.Error("missing delivery key")
		}
		if hits.Add(1) == 1 {
			w.WriteHeader(503)
		} else {
			w.WriteHeader(204)
		}
	}))
	defer server.Close()
	_ = setting.UpdateAlert(setting.OptionKeyAlertWebhookURL, server.URL)
	DB.Model(&User{}).Where("id = ?", u.ID).Update("quota_nano_remainder", 1)
	reconciliationReport(t, u, day, now)
	reconciliationReport(t, u, day, now)
	var count int64
	DB.Model(&ReconciliationAlert{}).Count(&count)
	if count != 1 {
		t.Fatal("duplicate alert", count)
	}
	if err := DeliverReconciliationAlerts(ctx, now); err != nil {
		t.Fatal(err)
	}
	if err := DeliverReconciliationAlerts(ctx, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	var alert ReconciliationAlert
	DB.First(&alert)
	if hits.Load() != 2 || alert.DeliveredAt == 0 {
		t.Fatal("alert retry failed")
	}
	DB.Model(&User{}).Where("id = ?", u.ID).Update("quota_nano_remainder", 0)
	reconciliationReport(t, u, day, now)
	DB.Model(&User{}).Where("id = ?", u.ID).Update("quota_nano_remainder", 1)
	reconciliationReport(t, u, day, now)
	DB.Model(&ReconciliationAlert{}).Count(&count)
	if count != 2 {
		t.Fatal("new recurrence did not alert")
	}
}

func TestPaymentStatementImportAndReconciliation(t *testing.T) {
	u, _, now := reconciliationFixture(t)
	ctx := context.Background()
	day := now.AddDate(0, 0, -1)
	csv := "provider_order_id,platform_order_id,user_id,amount_cents,currency,paid_at\n" + fmt.Sprintf("pay-1,platform-1,%d,1000,USD,%s\n", u.ID, day.Format(time.RFC3339))
	input := PaymentStatementImport{Provider: "bank-a", Day: day.Format("2006-01-02"), Source: "statement.csv", CSV: csv}
	batch, err := ImportPaymentStatement(ctx, input, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := ImportPaymentStatement(ctx, input, 0, now)
	if err != nil || duplicate.ID != batch.ID || batch.TotalNano != "10000000000" {
		t.Fatalf("idempotency or cents conversion: %+v %v", batch, err)
	}
	r := reconciliationReport(t, u, day, now)
	if !containsDifference(r, "provider_order_vs_ledger") || r.ExternalStatus != "imported_scope" {
		t.Fatal(r)
	}
	// Simulated credits cannot substantiate real imported payments.
	DB.Create(&LedgerEntry{UserID: u.ID, Type: "recharge", FundingSource: FundingSourceSimulation, OrderID: "platform-1", Amount: 10_000_000_000, CreatedAt: day.Unix()})
	if !containsDifference(reconciliationReport(t, u, day, now), "provider_order_vs_ledger") {
		t.Fatal("simulation verified a real payment")
	}
	DB.Create(&LedgerEntry{UserID: u.ID, Type: "recharge", FundingSource: FundingSourcePaymentProvider, OrderID: "platform-1", Amount: 10_000_000_000, CreatedAt: day.Unix()})
	if containsDifference(reconciliationReport(t, u, day, now), "provider_order_vs_ledger") {
		t.Fatal("matching imported payment failed")
	}
	input.CSV = strings.Replace(csv, ",1000,", ",1100,", 1)
	if _, err := ImportPaymentStatement(ctx, input, 0, now); err == nil {
		t.Fatal("stale statement overwrite accepted")
	}
	input.ExpectedPreviousID = batch.ID
	newer, err := ImportPaymentStatement(ctx, input, 0, now)
	if err != nil || newer.Supersedes != batch.ID {
		t.Fatalf("version import: %v", err)
	}
	var saved User
	DB.First(&saved, u.ID)
	if saved.Quota != u.Quota {
		t.Fatal("import changed customer funds")
	}
	var count int64
	DB.Model(&PaymentStatement{}).Count(&count)
	if count != 2 {
		t.Fatal("statement history lost")
	}
	for _, bad := range []string{strings.Replace(csv, ",1000,", ",10.5,", 1), strings.Replace(csv, ",USD,", ",EUR,", 1), csv + strings.Split(csv, "\n")[1] + "\n", strings.Replace(csv, ",1000,", fmt.Sprintf(",%d,", int64(math.MaxInt64)), 1)} {
		input.CSV = bad
		input.ExpectedPreviousID = newer.ID
		if _, err := ImportPaymentStatement(ctx, input, 0, now); err == nil {
			t.Fatal("invalid statement accepted")
		}
	}
}

func TestPaymentStatementTotalsExceedInt64(t *testing.T) {
	u, _, now := reconciliationFixture(t)
	day := now.AddDate(0, 0, -1)
	content := "provider_order_id,platform_order_id,user_id,amount_cents,currency,paid_at\n"
	for i := 0; i < 2; i++ {
		content += fmt.Sprintf("large-%d,,%d,900000000000,USD,%s\n", i, u.ID, day.Format(time.RFC3339))
	}
	batch, err := ImportPaymentStatement(context.Background(), PaymentStatementImport{Provider: "large-bank", Day: day.Format("2006-01-02"), Source: "large.csv", CSV: content}, 0, now)
	if err != nil || batch.TotalNano != "18000000000000000000" {
		t.Fatalf("large statement truncated: %+v %v", batch, err)
	}
}

func TestReconciliationScheduleDST(t *testing.T) {
	u, _, _ := reconciliationFixture(t)
	_ = u
	_ = config.SetRegionTimezone("America/New_York")
	loc, _ := time.LoadLocation("America/New_York")
	DB.Create(&ReconciliationSchedule{ID: 1, Timezone: loc.String(), CompletedDay: "2026-03-06"})
	before := time.Date(2026, 3, 8, 1, 59, 59, 0, loc)
	if err := RunScheduledReconciliation(context.Background(), before); err != nil {
		t.Fatal(err)
	}
	var state ReconciliationSchedule
	DB.First(&state, 1)
	if state.CompletedDay != "2026-03-06" {
		t.Fatal("DST gap ran the 02:00 job early")
	}
	if err := RunScheduledReconciliation(context.Background(), before.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	DB.First(&state, 1)
	if state.CompletedDay != "2026-03-07" {
		t.Fatal("DST gap skipped the daily job")
	}
}

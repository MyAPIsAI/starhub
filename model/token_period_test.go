package model

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
)

func periodicBillingToken(t *testing.T, limit int64) (*User, *Token) {
	t.Helper()
	u, token := newBillingUser(t, 10000)
	if err := UpdateTokenFields(token.ID, map[string]any{"quota_period": "daily", "period_quota_limit": limit}); err != nil {
		t.Fatal(err)
	}
	token, err := GetTokenByID(token.ID)
	if err != nil {
		t.Fatal(err)
	}
	return u, token
}

func TestTokenPeriodCalendarBoundariesAndRegionTimezone(t *testing.T) {
	for _, sample := range []struct {
		period, zone, at string
		hours            int64
	}{
		{"daily", "America/New_York", "2026-03-08T16:00:00Z", 23},
		{"daily", "America/New_York", "2026-11-01T16:00:00Z", 25},
		{"monthly", "Asia/Dubai", "2028-02-20T00:00:00Z", 29 * 24},
	} {
		now, _ := time.Parse(time.RFC3339, sample.at)
		start, end, err := tokenPeriodBounds(sample.period, sample.zone, now)
		if err != nil || end-start != sample.hours*3600 || now.Unix() < start || now.Unix() >= end {
			t.Fatalf("calendar boundary: %+v %d %d %v", sample, start, end, err)
		}
	}
	newTestDB(t)
	u, _ := newBillingUser(t, 1000)
	region := RegionProfile{ID: "dubai", Name: "Dubai", Timezone: "Asia/Dubai", Enabled: true}
	if err := DB.Create(&region).Error; err != nil {
		t.Fatal(err)
	}
	token := &Token{UserID: u.ID, Name: "regional", QuotaPeriod: "monthly", PeriodQuotaLimit: 100, QuotaRegion: "dubai"}
	if err := InsertToken(token); err != nil {
		t.Fatal(err)
	}
	if token.QuotaTimezone != "Asia/Dubai" || token.RemainQuota != 100 || token.PeriodQuotaLimit != 100 || token.QuotaPeriodID <= 0 {
		t.Fatalf("regional policy: %+v", token)
	}
	var periods int64
	DB.Model(&TokenQuotaPeriod{}).Where("token_id = ?", token.ID).Count(&periods)
	if periods != 1 {
		t.Fatal("initialization created an obsolete extra period")
	}
}

func TestTokenPeriodConcurrentReservationsAndRollover(t *testing.T) {
	newTestDB(t)
	u, token := periodicBillingToken(t, 1000)
	var wg sync.WaitGroup
	var accepted atomic.Int32
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := ReserveQuota(u.ID, token.ID, false, 100, fmt.Sprintf("attempt-%d", i))
			if err == nil {
				accepted.Add(1)
			} else if !errors.Is(err, ErrTokenQuotaShort) {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	got, _ := GetTokenByID(token.ID)
	if accepted.Load() != 10 || got.RemainQuota != 0 || got.PeriodReservedQuota != 1000 || got.UsedQuota != 0 {
		t.Fatalf("overspent period: accepted=%d token=%+v", accepted.Load(), got)
	}
	future := time.Unix(token.PeriodEnd+1, 0)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			copy := *token
			if err := RefreshTokenQuota(&copy, future); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	got, _ = GetTokenByID(token.ID)
	var count int64
	DB.Model(&TokenQuotaPeriod{}).Where("token_id = ?", token.ID).Count(&count)
	if count != 2 || got.RemainQuota != 1000 || got.PeriodUsedQuota != 0 || got.PeriodReservedQuota != 0 {
		t.Fatalf("duplicate or incomplete rollover: %+v count=%d", got, count)
	}
	owner, _ := GetUserByID(u.ID)
	if owner.Quota != 9000 {
		t.Fatal("rollover changed account funds")
	}
}

func TestTokenPeriodLateSettlementAndRefundDoNotRefillNextPeriod(t *testing.T) {
	newTestDB(t)
	u, token := periodicBillingToken(t, 1000)
	if err := ReserveQuota(u.ID, token.ID, false, 800, "old-paid"); err != nil {
		t.Fatal(err)
	}
	if err := ReserveQuota(u.ID, token.ID, false, 100, "old-refund"); err != nil {
		t.Fatal(err)
	}
	oldID := token.QuotaPeriodID
	if err := RefreshTokenQuota(token, time.Unix(token.PeriodEnd+1, 0)); err != nil {
		t.Fatal(err)
	}
	if err := ReserveQuota(u.ID, token.ID, false, 700, "new-pending"); err != nil {
		t.Fatal(err)
	}
	entry := &Log{Model: "late", QuotaReservationID: "old-paid"}
	if err := SettleQuota(u.ID, token.ID, false, 300, 1100, entry); err != nil {
		t.Fatal(err)
	}
	if entry.Quota != 900 || entry.UnfundedQuota != 200 || entry.QuotaPeriodID != oldID || entry.AdmittedAt == 0 {
		t.Fatalf("wrong old-period settlement: %+v", entry)
	}
	for i := 0; i < 2; i++ {
		if err := ReleaseQuota(u.ID, token.ID, false, 100, "old-refund"); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := GetTokenByID(token.ID)
	if got.RemainQuota != 300 || got.PeriodReservedQuota != 700 || got.PeriodUsedQuota != 0 || got.UsedQuota != 900 {
		t.Fatalf("late result touched new allowance: %+v", got)
	}
	var old TokenQuotaPeriod
	DB.First(&old, oldID)
	if old.UsedQuota != 900 || old.ReservedQuota != 0 {
		t.Fatalf("old history lost usage: %+v", old)
	}
	owner, _ := GetUserByID(u.ID)
	if owner.Quota != 8400 || owner.UsedQuota != 900 {
		t.Fatalf("incorrect account settlement: %+v", owner)
	}
}

func TestTokenPeriodDurableSettlementSurvivesRestartAndRollsBackWithLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "period.db")
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	u, token := periodicBillingToken(t, 1000)
	if err := ReserveQuota(u.ID, token.ID, false, 80, "durable-period"); err != nil {
		t.Fatal(err)
	}
	intent, err := RecordQuotaSettlement(u.ID, token.ID, false, -30, 50, &Log{Model: "billed", QuotaReservationID: "durable-period"})
	if err != nil {
		t.Fatal(err)
	}
	if err := RefreshTokenQuota(token, time.Unix(token.PeriodEnd+1, 0)); err != nil {
		t.Fatal(err)
	}
	if err := DB.Callback().Create().Before("gorm:create").Register("test:period_log_failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "logs" {
			tx.AddError(errors.New("synthetic log failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ApplyQuotaSettlement(intent.ID); err == nil {
		t.Fatal("log failure was ignored")
	}
	var r QuotaReservation
	DB.First(&r, "id = ?", "durable-period")
	if r.CompletedAt != 0 {
		t.Fatal("failed transaction completed reservation")
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, claimed, err := ApplyQuotaSettlement(intent.ID); err != nil || claimed != (i == 0) {
			t.Fatalf("durable retry: %v %v", claimed, err)
		}
	}
	got, _ := GetTokenByID(token.ID)
	owner, _ := GetUserByID(u.ID)
	if got.RemainQuota != 1000 || got.UsedQuota != 50 || owner.Quota != 9950 {
		t.Fatalf("restart settlement: %+v %+v", got, owner)
	}
}

func TestTokenPeriodPolicyChangesRespectReservationsAndSpentQuota(t *testing.T) {
	newTestDB(t)
	u, token := periodicBillingToken(t, 1000)
	if err := ReserveQuota(u.ID, token.ID, false, 80, "pending-edit"); err != nil {
		t.Fatal(err)
	}
	if err := UpdateTokenFields(token.ID, map[string]any{"quota_period": "monthly"}); !errors.Is(err, ErrTokenPeriodBusy) {
		t.Fatalf("changed a pending policy: %v", err)
	}
	if err := UpdateTokenFields(token.ID, map[string]any{"period_quota_limit": int64(79)}); !errors.Is(err, ErrTokenPeriodPolicy) {
		t.Fatalf("limit ignored held quota: %v", err)
	}
	if err := SettleQuota(u.ID, token.ID, false, -30, 50, &Log{QuotaReservationID: "pending-edit"}); err != nil {
		t.Fatal(err)
	}
	if err := UpdateTokenFields(token.ID, map[string]any{"quota_period": "monthly"}); err != nil {
		t.Fatal(err)
	}
	got, _ := GetTokenByID(token.ID)
	if got.RemainQuota != 950 || got.PeriodUsedQuota != 50 || got.UsedQuota != 50 {
		t.Fatalf("schedule change erased spent quota: %+v", got)
	}
	if err := ReserveQuota(u.ID, token.ID, false, 1); !errors.Is(err, ErrQuotaReservation) {
		t.Fatalf("periodic quota bypassed receipt requirement: %v", err)
	}
}

func TestTokenPeriodAsyncTaskKeepsAdmissionPeriod(t *testing.T) {
	newTestDB(t)
	u, token := periodicBillingToken(t, 1000)
	if err := ReserveQuota(u.ID, token.ID, false, 80, "async-period"); err != nil {
		t.Fatal(err)
	}
	task := &Task{UserID: u.ID, TokenID: token.ID, Quota: 80, QuotaReservationID: "async-period", Status: "submitted"}
	if err := InsertTask(task); err != nil {
		t.Fatal(err)
	}
	if err := RefreshTokenQuota(token, time.Unix(token.PeriodEnd+1, 0)); err != nil {
		t.Fatal(err)
	}
	task, err := RecordTaskSettlement(task.ID, "succeeded", "", "{}", 30)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if claimed, err := SettleTaskQuota(task, &Log{Model: "async"}); err != nil || claimed != (i == 0) {
			t.Fatalf("task settlement: %v %v", claimed, err)
		}
	}
	got, _ := GetTokenByID(token.ID)
	if got.RemainQuota != 1000 || got.UsedQuota != 30 {
		t.Fatalf("late task changed current period: %+v", got)
	}
}

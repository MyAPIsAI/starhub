package model

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"starhub/common"
)

func TestNanoReservationsTargetsAreIdempotentAndSettleOneNano(t *testing.T) {
	newTestDB(t)
	u, token := newBillingUser(t, 1000)
	id := "n2:one-nano"
	for i := 0; i < 2; i++ {
		if err := ReserveQuota(u.ID, token.ID, false, 100, id); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		if err := ReserveQuota(u.ID, token.ID, false, 50, id, "target:150"); err != nil {
			t.Fatal(err)
		}
	}
	entry := &Log{QuotaReservationID: id, AmountUnit: common.MoneyUnitNano}
	if err := SettleQuota(u.ID, token.ID, false, -149, 1, entry); err != nil {
		t.Fatal(err)
	}
	u, _ = GetUserByID(u.ID)
	if common.QuotaMoney(u.Quota, u.QuotaRemainderNano).String() != "1999999" || entry.Quota != 0 || entry.QuotaRemainderNano != 1 {
		t.Fatal("fractional charge or reservation retry lost money")
	}
	if err := VerifyMoneyLedger(u.ID); err != nil {
		t.Fatal(err)
	}
}

func TestNanoAndLegacyMovementsShareOneReconstructibleJournal(t *testing.T) {
	newTestDB(t)
	u, token := newBillingUser(t, 1000)
	if err := ReserveQuota(u.ID, token.ID, false, 100, "n2:mixed"); err != nil {
		t.Fatal(err)
	}
	if err := SettleQuota(u.ID, token.ID, false, -99, 1, &Log{QuotaReservationID: "n2:mixed", AmountUnit: common.MoneyUnitNano}); err != nil {
		t.Fatal(err)
	}
	u, _ = GetUserByID(u.ID)
	if _, err := AdjustUserQuota(u.ID, 10, u.Quota, 1, "Mixed-version credit"); err != nil {
		t.Fatal(err)
	}
	if err := ReserveQuota(u.ID, token.ID, false, 10, "legacy-mixed"); err != nil {
		t.Fatal(err)
	}
	if err := SettleQuota(u.ID, token.ID, false, -9, 1, &Log{QuotaReservationID: "legacy-mixed"}); err != nil {
		t.Fatal(err)
	}
	if err := VerifyMoneyLedger(u.ID); err != nil {
		t.Fatal(err)
	}
	u, _ = GetUserByID(u.ID)
	if common.QuotaMoney(u.Quota, u.QuotaRemainderNano).String() != "2017999" {
		t.Fatal("legacy path discarded nano remainder")
	}
}

func TestNanoPeriodicLateSettlementAndReleaseNeverRefillNewPeriod(t *testing.T) {
	newTestDB(t)
	u, token := periodicBillingToken(t, 1)
	old := token.QuotaPeriodID
	if err := ReserveQuota(u.ID, token.ID, false, 500, "n2:late"); err != nil {
		t.Fatal(err)
	}
	if err := ReserveQuota(u.ID, token.ID, false, 100, "n2:refund"); err != nil {
		t.Fatal(err)
	}
	if err := RefreshTokenQuota(token, time.Unix(token.PeriodEnd+1, 0)); err != nil {
		t.Fatal(err)
	}
	if err := SettleQuota(u.ID, token.ID, false, 1, 501, &Log{QuotaReservationID: "n2:late", AmountUnit: common.MoneyUnitNano}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := ReleaseQuota(u.ID, token.ID, false, 100, "n2:refund"); err != nil {
			t.Fatal(err)
		}
	}
	current, _ := GetTokenByID(token.ID)
	var history TokenQuotaPeriod
	DB.First(&history, old)
	if common.QuotaMoney(current.RemainQuota, current.RemainQuotaRemainderNano).String() != "2000" || current.PeriodUsedQuotaRemainderNano != 0 || history.UsedQuotaRemainderNano != 501 || history.ReservedQuotaRemainderNano != 0 {
		t.Fatal("late money affected new allowance")
	}
	if err := VerifyMoneyLedger(u.ID); err != nil {
		t.Fatal(err)
	}
}

func TestNanoConcurrentReservationsAndDurableSettlementRollback(t *testing.T) {
	newTestDB(t)
	u, token := periodicBillingToken(t, 1)
	var wg sync.WaitGroup
	var accepted atomic.Int32
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := ReserveQuota(u.ID, token.ID, false, 100, fmt.Sprintf("n2:parallel-%d", i)); err == nil {
				accepted.Add(1)
			} else if err != ErrTokenQuotaShort {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	if accepted.Load() != 20 {
		t.Fatalf("nano budget admissions=%d", accepted.Load())
	}
	if err := VerifyMoneyLedger(u.ID); err != nil {
		t.Fatal(err)
	}
	var reservation QuotaReservation
	DB.Where("completed_at = 0").First(&reservation)
	intent, err := RecordQuotaSettlement(u.ID, token.ID, false, -99, 1, &Log{QuotaReservationID: reservation.ID, AmountUnit: common.MoneyUnitNano})
	if err != nil {
		t.Fatal(err)
	}
	DB.Exec(`CREATE TRIGGER fail_nano_log BEFORE INSERT ON logs BEGIN SELECT RAISE(ABORT,'injected'); END`)
	if _, _, err := ApplyQuotaSettlement(intent.ID); err == nil {
		t.Fatal("failed log did not roll back nano settlement")
	}
	if err := VerifyMoneyLedger(u.ID); err != nil {
		t.Fatal(err)
	}
	DB.Exec(`DROP TRIGGER fail_nano_log`)
	entry, claimed, err := ApplyQuotaSettlement(intent.ID)
	if err != nil || !claimed || entry.QuotaRemainderNano != 1 {
		t.Fatalf("nano retry: %+v %v", entry, err)
	}
	if _, claimed, err := ApplyQuotaSettlement(intent.ID); err != nil || claimed {
		t.Fatal("nano intent charged twice")
	}
	if err := VerifyMoneyLedger(u.ID); err != nil {
		t.Fatal(err)
	}
}

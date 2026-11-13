package model

import (
	"testing"
	"time"
)

func TestDailyReconciliationIsIdempotentAndRevisesLateAdmission(t *testing.T) {
	newTestDB(t)
	u, token := newBillingUser(t, 1000)
	day, _ := BusinessDayForDate("2026-11-10", "Asia/Dubai", "ae", 3)
	// An opening lifetime counter belongs to no fabricated daily expense.
	DB.Model(&User{}).Where("id = ?", u.ID).UpdateColumn("used_quota", 42)
	if _, err := ReadMoneyAccount(u.ID); err != nil {
		t.Fatal(err)
	}
	first, changed, err := ReconcileBusinessDay(u.ID, day)
	if err != nil || !changed || first.State != "matched" || first.LogQuotaNanoSum != "0" {
		t.Fatalf("%+v %v", first, err)
	}
	again, changed, err := ReconcileBusinessDay(u.ID, day)
	if err != nil || changed || again.ID != first.ID || again.Revision != 1 {
		t.Fatalf("rerun %+v %v", again, err)
	}
	if err := ReserveQuota(u.ID, token.ID, false, 100, "n2:late-reconciliation"); err != nil {
		t.Fatal(err)
	}
	if err := DB.Model(&QuotaReservation{}).Where("id = ?", "n2:late-reconciliation").Updates(map[string]any{"business_calendar_version": day.BusinessCalendarVersion, "business_date": day.BusinessDate, "business_timezone": day.BusinessTimezone, "business_region": day.BusinessRegion, "business_region_revision": 3, "business_start": day.BusinessStart, "business_end": day.BusinessEnd, "admitted_at": day.BusinessStart + 30}).Error; err != nil {
		t.Fatal(err)
	}
	if err := SettleQuota(u.ID, token.ID, false, -99, 1, &Log{QuotaReservationID: "n2:late-reconciliation", Status: 1}); err != nil {
		t.Fatal(err)
	}
	late, changed, err := ReconcileBusinessDay(u.ID, day)
	if err != nil || !changed || late.ID != first.ID || late.Revision != 2 || late.State != "matched" || late.LogQuotaNanoSum != "1" || late.LedgerSpentNano != "1" {
		t.Fatalf("late %+v %v", late, err)
	}
	days, err := ReconciliationDays(u.ID, time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range days {
		if d.BusinessDate == day.BusinessDate && d.BusinessTimezone == day.BusinessTimezone {
			found = true
		}
	}
	if !found {
		t.Fatal("late original day was not scheduled")
	}
	var entry Log
	DB.First(&entry, "quota_reservation_id = ?", "n2:late-reconciliation")
	DB.Model(&entry).UpdateColumn("quota_remainder_nano", 2)
	drift, changed, err := ReconcileBusinessDay(u.ID, day)
	if err != nil || !changed || drift.Revision != 3 || drift.DifferenceNano != "1" || !drift.Alerted {
		t.Fatalf("drift %+v %v", drift, err)
	}
	_, changed, err = ReconcileBusinessDay(u.ID, day)
	if err != nil || changed {
		t.Fatal("unchanged drift alerted twice", err)
	}
}

func TestRegionalReconciliationWaitsForLocalTwoAM(t *testing.T) {
	newTestDB(t)
	u, _ := newBillingUser(t, 1000)
	day, _ := BusinessDayForDate("2026-11-10", "Asia/Dubai", "ae", 1)
	if err := InsertLog(&Log{UserID: u.ID, BusinessDaySnapshot: day, Status: 2}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		after int64
		want  bool
	}{{3600, false}, {7200, true}} {
		days, err := ReconciliationDays(u.ID, time.Unix(day.BusinessEnd+tc.after, 0))
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, d := range days {
			if d.BusinessRegion == "ae" {
				found = true
			}
		}
		if found != tc.want {
			t.Fatalf("after %d: %+v", tc.after, days)
		}
	}
}

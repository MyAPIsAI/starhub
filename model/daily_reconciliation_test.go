package model

import (
	"sync"
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

func TestLocalReconciliationTwoAMAcrossDSTGapAndFold(t *testing.T) {
	newTestDB(t)
	user, _ := newBillingUser(t, 1000)
	for _, tc := range []struct{ date, cutoff string }{{"2026-03-07", "2026-03-08T07:00:00Z"}, {"2026-10-31", "2026-11-01T07:00:00Z"}} {
		day, err := BusinessDayForDate(tc.date, "America/New_York", "us", 1)
		if err != nil {
			t.Fatal(err)
		}
		if err := InsertLog(&Log{UserID: user.ID, BusinessDaySnapshot: day, Status: 2}); err != nil {
			t.Fatal(err)
		}
		cutoff, err := time.Parse(time.RFC3339, tc.cutoff)
		if err != nil {
			t.Fatal(err)
		}
		for _, offset := range []time.Duration{-time.Second, 0} {
			days, err := ReconciliationDays(user.ID, cutoff.Add(offset))
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, candidate := range days {
				if candidate.BusinessDate == tc.date && candidate.BusinessRegion == "us" {
					found = true
				}
			}
			if found != (offset == 0) {
				t.Fatalf("date %s offset %s eligibility=%v", tc.date, offset, found)
			}
		}
	}
}

func TestConcurrentReconciliationKeepsOneReportAndOneRevision(t *testing.T) {
	newTestDB(t)
	u, _ := newBillingUser(t, 1000)
	day, _ := BusinessDayForDate("2026-11-10", "UTC", "", 0)
	var wg sync.WaitGroup
	errors := make(chan error, 8)
	changes := make(chan bool, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, changed, err := ReconcileBusinessDay(u.ID, day)
			errors <- err
			changes <- changed
		}()
	}
	wg.Wait()
	close(errors)
	close(changes)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	n := 0
	for changed := range changes {
		if changed {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("created %d revisions", n)
	}
	var reports, revisions int64
	DB.Model(&ReconciliationRecord{}).Count(&reports)
	DB.Model(&ReconciliationRevision{}).Count(&revisions)
	if reports != 1 || revisions != 1 {
		t.Fatalf("reports=%d revisions=%d", reports, revisions)
	}
}

func TestManualReconciliationAuditFailureRollsBackReportAndHistory(t *testing.T) {
	newTestDB(t)
	u, _ := newBillingUser(t, 1000)
	day, _ := BusinessDayForDate("2026-11-10", "UTC", "", 0)
	if err := DB.Exec("CREATE TRIGGER reject_reconciliation_audit BEFORE INSERT ON audit_logs BEGIN SELECT RAISE(ABORT, 'audit unavailable'); END").Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReconcileBusinessDay(u.ID, day, AuditLog{Username: "test-operator", Reason: "Verify original admission day"}); err == nil {
		t.Fatal("failed audit was reported as success")
	}
	var reports, revisions int64
	DB.Model(&ReconciliationRecord{}).Count(&reports)
	DB.Model(&ReconciliationRevision{}).Count(&revisions)
	if reports != 0 || revisions != 0 {
		t.Fatal("report persisted without audit")
	}
	DB.Exec("DROP TRIGGER reject_reconciliation_audit")
	for i := 0; i < 2; i++ {
		report, _, err := ReconcileBusinessDay(u.ID, day, AuditLog{Username: "test-operator", Reason: "Verify original admission day"})
		if err != nil || report.Revision != 1 {
			t.Fatal(report, err)
		}
	}
	var audits int64
	DB.Model(&AuditLog{}).Where("username = ?", "test-operator").Count(&audits)
	if audits != 2 {
		t.Fatal("manual attempts were not individually audited")
	}
}

func TestReconciliationRetainsSpendWhenLogIsMissing(t *testing.T) {
	newTestDB(t)
	u, token := newBillingUser(t, 1000)
	if err := ReserveQuota(u.ID, token.ID, false, 100, "n2:missing-log"); err != nil {
		t.Fatal(err)
	}
	entry := &Log{QuotaReservationID: "n2:missing-log", Status: 1}
	if err := SettleQuota(u.ID, token.ID, false, -99, 1, entry); err != nil {
		t.Fatal(err)
	}
	if err := DB.Delete(entry).Error; err != nil {
		t.Fatal(err)
	}
	report, _, err := ReconcileBusinessDay(u.ID, entry.BusinessDaySnapshot)
	if err != nil || report.LedgerSpentNano != "1" || report.LogQuotaNanoSum != "0" || report.DifferenceNano != "1" || report.UnverifiableLogs != 1 || !report.Alerted {
		t.Fatalf("%+v %v", report, err)
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

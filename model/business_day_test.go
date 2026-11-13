package model

import (
	"starhub/common"
	"testing"
	"time"
)

func TestBusinessDaysRetainRegionalBoundariesAndDSTLengths(t *testing.T) {
	day, err := BusinessDayForDate("2026-11-14", "Asia/Dubai", "ae", 3)
	if err != nil {
		t.Fatal(err)
	}
	if got := time.Unix(day.BusinessStart, 0).UTC().Format(time.RFC3339); got != "2026-11-13T20:00:00Z" {
		t.Fatalf("Dubai day start: %s", got)
	}
	for _, tc := range []struct {
		date  string
		hours int64
	}{{"2026-03-08", 23}, {"2026-11-01", 25}} {
		day, err := BusinessDayForDate(tc.date, "America/New_York", "us", 4)
		if err != nil || day.BusinessEnd-day.BusinessStart != tc.hours*3600 {
			t.Fatalf("DST day %s: %+v %v", tc.date, day, err)
		}
	}
	legacy := legacyBusinessDay(day.BusinessStart)
	if legacy.BusinessTimezone != "UTC" || legacy.BusinessCalendarVersion != 0 {
		t.Fatal("legacy admission adopted a new regional contract")
	}
}

func TestSettlementKeepsAdmissionCalendarAfterRegionChange(t *testing.T) {
	newTestDB(t)
	t.Setenv("STARHUB_REGION_PROFILE", "default")
	profile := RegionProfile{ID: "default", Timezone: "Asia/Dubai", Revision: 7, Enabled: true}
	if err := DB.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	u, token := newBillingUser(t, 1000)
	if err := ReserveQuota(u.ID, token.ID, false, 100, "n2:calendar"); err != nil {
		t.Fatal(err)
	}
	var receipt QuotaReservation
	if err := DB.First(&receipt, "id = ?", "n2:calendar").Error; err != nil {
		t.Fatal(err)
	}
	if receipt.BusinessTimezone != "Asia/Dubai" || receipt.BusinessRegionRevision != 7 {
		t.Fatal(receipt)
	}
	if err := DB.Model(&profile).Updates(map[string]any{"timezone": "America/New_York", "revision": 8}).Error; err != nil {
		t.Fatal(err)
	}
	entry := &Log{QuotaReservationID: receipt.ID, AmountUnit: common.MoneyUnitNano}
	if err := SettleQuota(u.ID, token.ID, false, -99, 1, entry); err != nil {
		t.Fatal(err)
	}
	if entry.BusinessDaySnapshot != receipt.BusinessDaySnapshot {
		t.Fatalf("calendar changed during settlement: %+v", entry.BusinessDaySnapshot)
	}
	var saved Log
	if err := DB.First(&saved, entry.ID).Error; err != nil {
		t.Fatal(err)
	}
	if saved.BusinessDaySnapshot != receipt.BusinessDaySnapshot {
		t.Fatal("calendar was not durable")
	}
}

func TestLegacyAdmissionKeepsUTCWhenDeploymentAdoptsRegionalCalendar(t *testing.T) {
	newTestDB(t)
	t.Setenv("STARHUB_REGION_PROFILE", "")
	user, token := newBillingUser(t, 1000)
	if err := ReserveQuota(user.ID, token.ID, false, 10, "legacy-calendar"); err != nil {
		t.Fatal(err)
	}
	admitted := time.Date(2026, 11, 11, 23, 30, 0, 0, time.UTC).Unix()
	if err := DB.Model(&QuotaReservation{}).Where("id = ?", "legacy-calendar").Updates(map[string]any{"business_calendar_version": 0, "business_date": "", "business_timezone": "", "business_region": "", "business_region_revision": 0, "business_start": 0, "business_end": 0, "admitted_at": admitted}).Error; err != nil {
		t.Fatal(err)
	}
	profile := &RegionProfile{ID: "new-calendar", Name: "New calendar", Timezone: "Asia/Dubai", Enabled: true}
	if err := SaveRegionProfile(profile, -1); err != nil {
		t.Fatal(err)
	}
	t.Setenv("STARHUB_REGION_PROFILE", profile.ID)
	entry := &Log{QuotaReservationID: "legacy-calendar", Status: 1}
	if err := SettleQuota(user.ID, token.ID, false, -9, 1, entry); err != nil {
		t.Fatal(err)
	}
	if entry.BusinessTimezone != "UTC" || entry.BusinessCalendarVersion != 0 || entry.BusinessDate != "2026-11-11" || entry.AdmittedAt != admitted {
		t.Fatalf("legacy calendar was reinterpreted: %+v", entry.BusinessDaySnapshot)
	}
}

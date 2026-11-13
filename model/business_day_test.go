package model

import (
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

package model

import (
	"testing"
	"time"
)

func TestUsageReportPreservesBothHoursAtDSTFallback(t *testing.T) {
	newTestDB(t)
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, loc)
	for _, stamp := range []string{"2026-11-01T01:30:00-04:00", "2026-11-01T01:30:00-05:00"} {
		at, err := time.Parse(time.RFC3339, stamp)
		if err != nil {
			t.Fatal(err)
		}
		if err := InsertLog(&Log{UserID: 1, Model: "m", CreatedAt: at.Unix(), Quota: 1}); err != nil {
			t.Fatal(err)
		}
	}
	f := LogFilters{Since: start.Unix(), Until: start.AddDate(0, 0, 1).Unix()}
	hourly, err := AggregateUsage(DB, 1, f, loc, "model", "hour")
	if err != nil {
		t.Fatal(err)
	}
	if len(hourly.Series) != 25 || hourly.Totals.Requests != 2 || hourly.Totals.AmountNano != "4000" {
		t.Fatalf("hourly totals: %+v", hourly)
	}
	var occupied []string
	for i, row := range hourly.Series {
		if i > 0 {
			previous, _ := time.Parse(time.RFC3339, hourly.Series[i-1].ID)
			at, _ := time.Parse(time.RFC3339, row.ID)
			if !previous.Before(at) {
				t.Fatal("hours are not chronological")
			}
		}
		if row.Requests > 0 {
			occupied = append(occupied, row.ID)
		}
	}
	if len(occupied) != 2 || occupied[0] != "2026-11-01T01:00:00-04:00" || occupied[1] != "2026-11-01T01:00:00-05:00" {
		t.Fatalf("repeated hours merged: %v", occupied)
	}
	daily, err := AggregateUsage(DB, 1, f, loc, "model", "day")
	if err != nil || len(daily.Series) != 1 || daily.Series[0].Requests != 2 || daily.Series[0].ID != "2026-11-01T00:00:00-04:00" {
		t.Fatalf("regional daily bucket: %+v %v", daily, err)
	}
}

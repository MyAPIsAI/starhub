package model

import (
	"testing"
	"time"
)

func TestAggregateMonthlyAvailability(t *testing.T) {
	newTestDB(t)
	now := time.Now().UTC()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	// Insert test logs: 8 success, 2 failed, all with channel_id > 0.
	for i := 0; i < 8; i++ {
		InsertLog(&Log{UserID: 1, Model: "gpt-4o", Status: 1, ChannelID: 1, CreatedAt: monthStart.Unix() + int64(i*3600)})
	}
	for i := 0; i < 2; i++ {
		InsertLog(&Log{UserID: 1, Model: "gpt-4o", Status: 2, ChannelID: 1, CreatedAt: monthStart.Unix() + int64(i*3600+100)})
	}
	// Insert a cancelled log (Status=3) — should be excluded.
	InsertLog(&Log{UserID: 1, Model: "gpt-4o", Status: 3, ChannelID: 1, CreatedAt: monthStart.Unix()})
	// Insert a log without channel — should be excluded.
	InsertLog(&Log{UserID: 1, Model: "gpt-4o", Status: 1, ChannelID: 0, CreatedAt: monthStart.Unix()})

	records, err := AggregateMonthlyAvailability(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	r := records[0]
	if r.SuccessCount != 8 || r.FailedCount != 2 || r.TotalRequests != 10 {
		t.Fatalf("counts wrong: success=%d failed=%d total=%d", r.SuccessCount, r.FailedCount, r.TotalRequests)
	}
	expectedPct := 80.0
	if r.UptimePercent != expectedPct {
		t.Fatalf("uptime = %f, want %f", r.UptimePercent, expectedPct)
	}
}

func TestAggregateMonthlyAvailabilityNoData(t *testing.T) {
	newTestDB(t)
	records, err := AggregateMonthlyAvailability(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record even with no data, got %d", len(records))
	}
	if records[0].TotalRequests != 0 || records[0].UptimePercent != 0 {
		t.Fatalf("expected zero counts, got %+v", records[0])
	}
}

func TestSaveAndListAvailabilityRecords(t *testing.T) {
	newTestDB(t)
	records := []AvailabilityRecord{
		{Month: "2026-09", Component: "overall", TotalRequests: 1000, SuccessCount: 990, FailedCount: 10, UptimePercent: 99.0, CalculatedAt: time.Now().Unix()},
		{Month: "2026-10", Component: "overall", TotalRequests: 2000, SuccessCount: 1990, FailedCount: 10, UptimePercent: 99.5, CalculatedAt: time.Now().Unix()},
	}
	if err := SaveAvailabilityRecords(records); err != nil {
		t.Fatal(err)
	}

	list, err := ListAvailabilityHistory(3)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 records, got %d", len(list))
	}
}

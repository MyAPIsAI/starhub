package model

import (
	"testing"
	"time"
)

func TestDailyUsageUsesFrozenRegionalAdmissionDay(t *testing.T) {
	newTestDB(t)
	day, _ := BusinessDayForDate("2026-11-14", "Asia/Dubai", "ae", 2)
	entry := Log{BusinessDaySnapshot: day, UserID: 1, Status: 1, Quota: 10, AdmittedAt: day.BusinessStart + 3600, CreatedAt: day.BusinessEnd + 72*3600}
	if err := InsertLog(&entry); err != nil {
		t.Fatal(err)
	}
	rows, err := SummarizeLogsByDay(1, day.BusinessStart)
	if err != nil || len(rows) != 1 || rows[0].Date != "2026-11-14" || rows[0].Quota != 10 {
		t.Fatalf("%+v %v", rows, err)
	}
	rows, err = SummarizeLogsByDay(1, time.Unix(day.BusinessEnd, 0).Unix())
	if err != nil || len(rows) != 0 {
		t.Fatalf("late completion leaked into later admission window: %+v %v", rows, err)
	}
}

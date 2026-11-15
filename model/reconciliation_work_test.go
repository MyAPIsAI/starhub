package model

import (
	"context"
	"testing"
	"time"
)

func TestDirtyReconciliationSurvivesLateWritesAndStaleAcknowledgment(t *testing.T) {
	newTestDB(t)
	day, _ := BusinessDayForDate("2026-11-10", "UTC", "", 0)
	ctx := context.Background()
	insert := func() {
		t.Helper()
		if err := InsertLog(&Log{UserID: 1, BusinessDaySnapshot: day, Status: 2}); err != nil {
			t.Fatal(err)
		}
	}
	insert()
	rows, err := PendingReconciliationWork(ctx, time.Date(2026, 11, 11, 1, 0, 0, 0, time.UTC))
	if err != nil || len(rows) != 0 {
		t.Fatal("day scheduled before 02:00", err)
	}
	rows, err = PendingReconciliationWork(ctx, time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC))
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	old := rows[0]
	insert()
	if err := AckReconciliationWork(ctx, old); err != nil {
		t.Fatal(err)
	}
	rows, err = PendingReconciliationWork(ctx, time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC))
	if err != nil || len(rows) != 1 || rows[0].Revision != 2 {
		t.Fatal("late work lost", rows, err)
	}
	if err := AckReconciliationWork(ctx, rows[0]); err != nil {
		t.Fatal(err)
	}
	rows, err = PendingReconciliationWork(ctx, time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC))
	if err != nil || len(rows) != 0 {
		t.Fatal("completed work remains pending", rows, err)
	}
}

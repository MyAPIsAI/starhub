package model

import (
	"strings"
	"testing"
)

func TestProductionCursorAndQueryPlans(t *testing.T) {
	newTestDB(t)
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		if err := DB.Create(&ProductionAttemptRecord{ID: id, Minute: 60, Payload: "{}"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	rows, next, err := ReadProductionAttemptPage(0, 120, "", 2)
	if err != nil || len(rows) != 2 || next == "" || rows[1].ID != "b" {
		t.Fatal(rows, next, err)
	}
	DB.Delete(&ProductionAttemptRecord{}, "id = ?", "a")
	rows, next, err = ReadProductionAttemptPage(0, 120, next, 2)
	if err != nil || len(rows) != 2 || next == "" || rows[0].ID != "c" {
		t.Fatal(rows, next, err)
	}
	rows, next, err = ReadProductionAttemptPage(0, 120, next, 2)
	if err != nil || len(rows) != 1 || next != "" || rows[0].ID != "e" {
		t.Fatal(rows, next, err)
	}
	if _, _, err := ReadProductionAttemptPage(0, 120, "invalid", 2); err != ErrProductionCursor {
		t.Fatal(err)
	}
	for _, tc := range []struct{ sql, index string }{
		{"SELECT id FROM production_attempt_records WHERE delivered_at=0 AND next_attempt_at<=120 ORDER BY next_attempt_at,minute,id LIMIT 64", "idx_production_pending"},
		{"SELECT id FROM production_attempt_records WHERE minute>=60 AND minute<=120 AND (minute>60 OR id>'b') ORDER BY minute,id LIMIT 65", "idx_production_minute_cursor"},
		{"SELECT id FROM production_attempt_records WHERE minute<120 ORDER BY minute,id LIMIT 512", "idx_production_minute_cursor"},
	} {
		var plan []struct{ Detail string }
		if err := DB.Raw("EXPLAIN QUERY PLAN " + tc.sql).Scan(&plan).Error; err != nil {
			t.Fatal(err)
		}
		text := ""
		for _, p := range plan {
			text += p.Detail + "\n"
		}
		if !strings.Contains(text, tc.index) || strings.Contains(text, "TEMP B-TREE") {
			t.Fatalf("unexpected query plan: %s", text)
		}
	}
}

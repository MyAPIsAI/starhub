package model

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestNetworkProductionCursorAndQueryPlans(t *testing.T) {
	for _, driver := range []string{"mysql", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			db := networkProductionDB(t, driver)
			rows := make([]ProductionAttemptRecord, 2000)
			for i := range rows {
				delivered := int64(1)
				if i%100 == 0 {
					delivered = 0
				}
				rows[i] = ProductionAttemptRecord{ID: fmt.Sprintf("%04d", i), Minute: int64(i/10+1) * 60, Payload: `{}`, DeliveredAt: delivered, NextAttemptAt: 120}
			}
			rows = append(rows, ProductionAttemptRecord{ID: "Case", Minute: 60, Payload: `{}`, DeliveredAt: 1}, ProductionAttemptRecord{ID: "case", Minute: 60, Payload: `{}`, DeliveredAt: 1})
			if err := db.CreateInBatches(&rows, 128).Error; err != nil {
				t.Fatal(err)
			}
			page, next, err := ReadProductionAttemptPage(0, 60, "", 11)
			if err != nil || len(page) != 11 || page[10].ID != "Case" || next == "" {
				t.Fatal("unstable binary cursor ordering", next, err)
			}
			if err := db.Delete(&ProductionAttemptRecord{}, "id=?", "0000").Error; err != nil {
				t.Fatal(err)
			}
			page, next, err = ReadProductionAttemptPage(0, 60, next, 11)
			if err != nil || len(page) != 1 || page[0].ID != "case" || next != "" {
				t.Fatal("cursor skipped or repeated rows after deletion", next, err)
			}
			analyze := "ANALYZE production_attempt_records"
			if driver == "mysql" {
				analyze = "ANALYZE TABLE production_attempt_records"
			}
			if err := db.Exec(analyze).Error; err != nil {
				t.Fatal(err)
			}
			for _, query := range []struct{ sql, index string }{
				{"SELECT id FROM production_attempt_records WHERE delivered_at=0 AND next_attempt_at<=120 ORDER BY next_attempt_at,minute,id LIMIT 64", "idx_production_pending"},
				{"SELECT id FROM production_attempt_records WHERE minute>=600 AND minute<=1200 ORDER BY minute,id LIMIT 65", "idx_production_minute_cursor"},
				{"SELECT id FROM production_attempt_records WHERE minute<120 ORDER BY minute,id LIMIT 512", "idx_production_minute_cursor"},
			} {
				prefix := "EXPLAIN (FORMAT JSON) "
				if driver == "mysql" {
					prefix = "EXPLAIN FORMAT=JSON "
				}
				var plan string
				if err := db.Raw(prefix + query.sql).Scan(&plan).Error; err != nil {
					t.Fatal(err)
				}
				var decoded any
				if json.Unmarshal([]byte(plan), &decoded) != nil || !productionPlanUsesIndex(decoded, query.index) {
					t.Fatalf("expected index %s missing: %s", query.index, plan)
				}
				t.Logf("%s: %s", query.index, plan)
			}
		})
	}
}

func productionPlanUsesIndex(value any, index string) bool {
	switch node := value.(type) {
	case map[string]any:
		if node["key"] == index || node["Index Name"] == index {
			return true
		}
		for _, child := range node {
			if productionPlanUsesIndex(child, index) {
				return true
			}
		}
	case []any:
		for _, child := range node {
			if productionPlanUsesIndex(child, index) {
				return true
			}
		}
	}
	return false
}

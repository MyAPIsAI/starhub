package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"starhub/metric"
	"starhub/model"
)

func TestProductionSpeedScopeProjection(t *testing.T) {
	resourceTestDB(t)
	now := time.Now().Truncate(time.Minute)
	at := now.Add(-time.Minute).Unix()
	b := metric.ProductionMinute{Minute: at, BatchID: "speed-scope", Complete: true, TotalAttempts: 6, Entries: []metric.ProductionEntry{{ChannelID: 1, ModelName: "glm-5.2", Generation: "g", Counts: metric.ProductionCounts{N: 6, S: 6}, Health: &metric.ProductionHealth{Contract: "confirmed-production-health-v1", N: 6, FirstToken: 6, InputCap: 2000}}}}
	var records []model.ProductionAttemptRecord
	for i, tokens := range []int64{1999, 2000, 2001, 19999, 20000, 20001} {
		a := metric.ProductionAttempt{AttemptMetadata: metric.AttemptMetadata{Generation: "g"}, ID: fmt.Sprintf("scope-%d", i), ChannelID: 1, Model: "glm-5.2", StartedAt: at * 1000, EndedAt: at*1000 + int64(i+1)*1000, Outcome: "success", Stream: true, InputTokens: tokens, FirstContentMS: int64(i+1) * 100}
		raw, _ := json.Marshal(a)
		records = append(records, model.ProductionAttemptRecord{ID: a.ID, Minute: at, Payload: string(raw)})
	}
	raw, _ := json.Marshal(b)
	row := model.ProductionTelemetryMinute{Minute: at, BatchID: b.BatchID, Complete: true, AttemptCount: 6, Payload: string(raw)}
	if err := model.SaveProductionTelemetry([]model.ProductionTelemetryMinute{row}, now, records...); err != nil {
		t.Fatal(err)
	}
	fixtures := map[string]any{}
	for _, tc := range []struct {
		cap   int
		count float64
	}{{2000, 2}, {20000, 5}} {
		rows, err := ProductionMetricBucketsScopeContext(context.Background(), at, at, now, tc.cap)
		if err != nil {
			t.Fatal(err)
		}
		data := rows[0]["models"].([]map[string]any)[0]
		var parts []map[string]any
		if err := json.Unmarshal([]byte(data["speed_evidence_json"].(string)), &parts); err != nil {
			t.Fatal(err)
		}
		if len(parts) != 2 || parts[1]["schema"] != float64(6) {
			t.Fatalf("missing schema 6: %+v", parts)
		}
		bucket := parts[1]["buckets"].([]any)[0].(map[string]any)
		if bucket["count"] != tc.count || data["ttft_count"] != tc.count {
			t.Fatalf("scope mismatch: %+v", data)
		}
		fixtures[fmt.Sprint(tc.cap)] = rows
	}
	var unchanged model.ProductionTelemetryMinute
	model.DB.First(&unchanged, at)
	if unchanged.Payload != row.Payload {
		t.Fatal("sealed manifest was modified")
	}
	if path := os.Getenv("STARHUB_SPEED_FIXTURE"); path != "" {
		raw, _ := json.Marshal(fixtures)
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	model.DB.Where("id = ?", records[0].ID).Delete(&model.ProductionAttemptRecord{})
	rows, err := ProductionMetricBucketsScopeContext(context.Background(), at, at, now, 1999)
	if err != nil {
		t.Fatal(err)
	}
	models := rows[0]["models"].([]map[string]any)
	if len(models) != 1 || models[0]["coverage_status"] != "attempt_retention_gap" {
		t.Fatal("missing journal asserted completeness")
	}
}

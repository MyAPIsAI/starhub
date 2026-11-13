package service

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"starhub/metric"
	"starhub/model"
)

func TestProductionTelemetryDurableRetryAndMissingIntervals(t *testing.T) {
	resourceTestDB(t)
	metric.Reset()
	t.Cleanup(metric.Reset)
	now := time.Now().Truncate(time.Minute)
	metric.StartProduction(now.Add(-time.Minute))
	s := metric.BeginRequest(metric.RequestMeta{Model: "m", Stream: true})
	a := s.BeginAttempt(7)
	a.SetRouteMetadata(metric.AttemptMetadata{Generation: "g", ResourceKey: "7:m", Capability: "chat", Group: "default", Affinity: "hit"})
	a.MarkSent()
	a.RecordUsage(10, 2)
	a.RecordFirstToken()
	s.EndAttempt(a, metric.AttemptSuccess, 200)
	if err := model.DB.Exec(`CREATE TRIGGER fail_telemetry BEFORE INSERT ON production_attempt_records BEGIN SELECT RAISE(ABORT,'injected'); END`).Error; err != nil {
		t.Fatal(err)
	}
	seal := now.Add(time.Minute)
	if err := FlushProductionTelemetry(seal); err == nil {
		t.Fatal("failed journal did not abort batch")
	}
	var count int64
	model.DB.Model(&model.ProductionTelemetryMinute{}).Count(&count)
	if count != 0 {
		t.Fatal("partial seal committed")
	}
	model.DB.Exec(`DROP TRIGGER fail_telemetry`)
	if err := FlushProductionTelemetry(seal); err != nil {
		t.Fatal(err)
	}
	if err := FlushProductionTelemetry(seal); err != nil {
		t.Fatal(err)
	}
	model.DB.Model(&model.ProductionAttemptRecord{}).Count(&count)
	if count != 1 {
		t.Fatal("attempt duplicated on retry")
	}
	rows, err := ProductionMetricBuckets(now.Add(-2*time.Minute).Unix(), seal.Unix(), seal)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 {
		t.Fatalf("minute sequence: %d", len(rows))
	}
	for i, complete := range []bool{false, false, true, false} {
		models := rows[i]["models"].([]map[string]any)
		marker := models[len(models)-1]
		var parts []map[string]any
		if err := json.Unmarshal([]byte(marker["speed_evidence_json"].(string)), &parts); err != nil {
			t.Fatal(err)
		}
		if (parts[0]["complete"] == true) != complete {
			t.Fatalf("wrong coverage: %+v", parts)
		}
	}
	if fixture := os.Getenv("STARHUB_PRODUCTION_FIXTURE"); fixture != "" {
		attempts, err := model.ReadProductionAttempts(now.Unix(), now.Unix(), 1, 100)
		if err != nil || len(attempts) != 1 {
			t.Fatal("attempt fixture unavailable")
		}
		raw, _ := json.Marshal(map[string]any{"buckets": rows, "attempt_batch": map[string]any{"schema": 1, "source": "talen", "instance_id": "fixture-node", "attempts": []json.RawMessage{json.RawMessage(attempts[0].Payload)}}})
		if err := os.WriteFile(fixture, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	metric.StartProduction(seal.Add(time.Minute))
	after, err := ProductionMetricBuckets(now.Unix(), now.Unix(), seal.Add(time.Minute))
	if err != nil || len(after) != 1 {
		t.Fatalf("restart lost sealed evidence: %v", err)
	}
}

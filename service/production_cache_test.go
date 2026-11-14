package service

import (
	"encoding/json"
	"starhub/metric"
	"starhub/model"
	"testing"
	"time"
)

func TestProductionCacheIsDetachedBoundedAndRejectsChangedCorruptSource(t *testing.T) {
	resourceTestDB(t)
	now := time.Now()
	minute := now.Truncate(time.Minute).Add(-time.Minute).Unix()
	b := metric.ProductionMinute{Minute: minute, BatchID: "cache-test", Complete: true, Entries: []metric.ProductionEntry{}, Attempts: []metric.ProductionAttempt{}}
	raw, _ := json.Marshal(b)
	row := model.ProductionTelemetryMinute{Minute: minute, BatchID: b.BatchID, Complete: true, Payload: string(raw)}
	if err := model.SaveProductionTelemetry([]model.ProductionTelemetryMinute{row}, now); err != nil {
		t.Fatal(err)
	}
	first, err := ProductionMetricBuckets(minute, minute, now)
	if err != nil {
		t.Fatal(err)
	}
	first[0]["models"].([]map[string]any)[0]["coverage_status"] = "mutated"
	hits := productionCacheHits.Load()
	second, err := ProductionMetricBuckets(minute, minute, now)
	if err != nil || productionCacheHits.Load() != hits+1 || second[0]["models"].([]map[string]any)[0]["coverage_status"] != "complete" {
		t.Fatal("cache was mutated or not reused", err)
	}
	if err := model.DB.Model(&model.ProductionTelemetryMinute{}).Where("minute = ?", minute).Update("payload", "corrupt").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := ProductionMetricBuckets(minute, minute, now); err == nil {
		t.Fatal("cached value hid corrupt changed source")
	}
	productionCache.Lock()
	defer productionCache.Unlock()
	if productionCache.bytes > 16<<20 || productionCache.order.Len() > 2048 {
		t.Fatal("cache exceeded budget")
	}
}

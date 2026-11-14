package model

import (
	"fmt"
	"testing"
	"time"
)

func TestRetentionReportsUnpublishedLossAndNeverPublishesCompleteCoverage(t *testing.T) {
	newTestDB(t)
	now := time.Now()
	minute := now.Truncate(time.Minute).Add(-time.Minute).Unix()
	if err := DB.Model(&ProductionStorageState{}).Where("id=1").Updates(map[string]any{"max_rows": 5, "max_bytes": 4096}).Error; err != nil {
		t.Fatal(err)
	}
	rows := make([]ProductionAttemptRecord, 5)
	for i := range rows {
		rows[i] = ProductionAttemptRecord{ID: fmt.Sprintf("retained-%d", i), BatchID: "retention-batch", Minute: minute, Payload: fmt.Sprintf(`{"attempt_id":"retained-%d"}`, i)}
	}
	if err := StageProductionAttempts(rows); err != nil {
		t.Fatal(err)
	}
	n, err := PruneProductionTelemetry(now)
	if err != nil || n != 1 {
		t.Fatal(n, err)
	}
	state, err := ReadProductionStorage()
	if err != nil || state.Rows != 4 || state.Dropped != 1 || state.UndeliveredDropped != 1 || state.LastDropReason != "capacity" {
		t.Fatal(state, err)
	}
	manifest := ProductionTelemetryMinute{Minute: minute, BatchID: "retention-batch", Complete: true, AttemptCount: 5, Payload: fmt.Sprintf(`{"minute":%d,"batch_id":"retention-batch","complete":true,"entries":[],"attempts":[]}`, minute)}
	for i := 0; i < 2; i++ {
		if err := SaveProductionTelemetry([]ProductionTelemetryMinute{manifest}, now); err != nil {
			t.Fatal(err)
		}
	}
	var stored ProductionTelemetryMinute
	if err := DB.First(&stored, minute).Error; err != nil || stored.Complete {
		t.Fatal("retention loss became complete coverage", err)
	}
	visible, err := ReadProductionAttempts(minute, minute, 1, 100)
	if err != nil || len(visible) != 4 {
		t.Fatal("retained attempts unavailable", err)
	}
}

func TestRetentionPrefersDeliveredAndAuditsAgeExpiration(t *testing.T) {
	newTestDB(t)
	now := time.Now()
	minute := now.Truncate(time.Minute).Unix()
	for _, row := range []ProductionAttemptRecord{{ID: "old-pending", Minute: minute - 25*3600, Payload: "old"}, {ID: "delivered", Minute: minute, DeliveredAt: now.Unix(), Payload: "delivered"}, {ID: "pending", Minute: minute, Payload: "pending"}} {
		if err := DB.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if n, err := PruneProductionTelemetry(now); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	state, _ := ReadProductionStorage()
	if state.LastDropReason != "age" || state.UndeliveredDropped != 1 {
		t.Fatal(state)
	}
	DB.Model(&ProductionStorageState{}).Where("id=1").Update("max_rows", 2)
	if n, err := PruneProductionTelemetry(now); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	state, _ = ReadProductionStorage()
	if state.UndeliveredDropped != 1 {
		t.Fatal("pending data evicted before delivered data", state)
	}
	var count int64
	DB.Model(&ProductionAttemptRecord{}).Where("id = ?", "pending").Count(&count)
	if count != 1 {
		t.Fatal("pending record not retained")
	}
}

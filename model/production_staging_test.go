package model

import (
	"fmt"
	"testing"
	"time"
)

func TestProductionStagingIsBoundedInvisibleAndPublishedAtomically(t *testing.T) {
	newTestDB(t)
	now := time.Now()
	minute := now.Truncate(time.Minute).Add(-time.Minute).Unix()
	rows := make([]ProductionAttemptRecord, 256)
	for i := range rows {
		rows[i] = ProductionAttemptRecord{ID: fmt.Sprintf("stage-%04d", i), Minute: minute, BatchID: "batch", Payload: fmt.Sprintf(`{"attempt_id":"stage-%04d"}`, i)}
	}
	if err := DB.Exec("CREATE TRIGGER fail_later_stage BEFORE INSERT ON production_attempt_records WHEN NEW.id = 'stage-0128' BEGIN SELECT RAISE(ABORT,'stage unavailable'); END").Error; err != nil {
		t.Fatal(err)
	}
	if err := StageProductionAttempts(rows); err == nil {
		t.Fatal("stage failure ignored")
	}
	var staged int64
	DB.Model(&ProductionAttemptRecord{}).Count(&staged)
	if staged != 128 {
		t.Fatalf("bounded first stage=%d", staged)
	}
	visible, err := ReadProductionAttempts(minute, minute, 1, 1000)
	if err != nil || len(visible) != 0 {
		t.Fatal("partial stage was published", err)
	}
	manifest := ProductionTelemetryMinute{Minute: minute, BatchID: "batch", Complete: true, AttemptCount: 256, Payload: "immutable manifest"}
	if err := SaveProductionTelemetry([]ProductionTelemetryMinute{manifest}, now); err == nil {
		t.Fatal("missing stages published as complete")
	}
	DB.Exec("DROP TRIGGER fail_later_stage")
	if err := StageProductionAttempts(rows); err != nil {
		t.Fatal(err)
	}
	if err := SaveProductionTelemetry([]ProductionTelemetryMinute{manifest}, now); err != nil {
		t.Fatal(err)
	}
	if err := SaveProductionTelemetry([]ProductionTelemetryMinute{manifest}, now); err != nil {
		t.Fatal("publication retry was not idempotent", err)
	}
	visible, err = ReadProductionAttempts(minute, minute, 1, 1000)
	if err != nil || len(visible) != 256 {
		t.Fatalf("visible=%d: %v", len(visible), err)
	}
	rows[0].Payload = "changed payload"
	if err := StageProductionAttempts(rows[:1]); err == nil {
		t.Fatal("stage identity accepted different payload")
	}
}

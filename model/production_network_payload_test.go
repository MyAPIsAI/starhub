package model

import (
	"strings"
	"testing"
	"time"
)

func TestNetworkProductionPayloadBoundaries(t *testing.T) {
	t.Setenv("STARHUB_TELEMETRY_DB_TIMEOUT", "30s")
	for _, driver := range []string{"mysql", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			db := networkProductionDB(t, driver)
			now := time.Now().Truncate(time.Minute)
			minute := now.Add(-time.Minute).Unix()
			payload := strings.Repeat("中🙂", (128<<10)/7) + strings.Repeat("x", (128<<10)%7)
			attempt := ProductionAttemptRecord{ID: "large", BatchID: "large-batch", Minute: minute, Payload: payload}
			if err := StageProductionAttempts([]ProductionAttemptRecord{attempt}); err != nil {
				t.Fatal("128 KiB UTF-8 attempt", err)
			}
			manifest := ProductionTelemetryMinute{Minute: minute, BatchID: "large-batch", Complete: true, AttemptCount: 1, Payload: strings.Repeat("x", 64<<20)}
			if err := publishProductionMinute(manifest, now); err != nil {
				t.Fatal("64 MiB minute", err)
			}
			var stored ProductionTelemetryMinute
			if err := db.First(&stored, minute).Error; err != nil || stored.Payload != manifest.Payload {
				t.Fatal("large minute did not round trip", err)
			}
			state, err := ReadProductionStorage()
			if err != nil || state.Bytes != 128<<10 || state.Rows != 1 {
				t.Fatal("large UTF-8 byte accounting", state, err)
			}
			if err := CompleteProductionDelivery([]ProductionAttemptRecord{attempt}, strings.Repeat("中", 200), now); err != nil {
				t.Fatal("bounded Unicode error receipt", err)
			}
			var receipt ProductionAttemptRecord
			if err := db.First(&receipt, "id=?", attempt.ID).Error; err != nil || len([]rune(receipt.LastError)) != 160 || receipt.Attempts != 1 {
				t.Fatal("invalid receipt", receipt.Attempts, err)
			}
		})
	}
}

package service

import (
	"encoding/json"
	"starhub/metric"
	"starhub/model"
	"strings"
	"testing"
	"time"
)

func TestProductionHealthReportsDurableBacklogAndPersistenceFailures(t *testing.T) {
	resourceTestDB(t)
	metric.Reset()
	t.Cleanup(metric.Reset)
	now := time.Now().Truncate(time.Minute)
	if err := model.DB.Create(&[]model.ProductionAttemptRecord{
		{ID: "pending", Minute: now.Add(-time.Minute).Unix(), Payload: `{"private":"never-expose-payload"}`},
		{ID: "staged", Minute: now.Unix(), BatchID: "unfinished", Payload: "{}"},
	}).Error; err != nil {
		t.Fatal(err)
	}
	h := ProductionTelemetryHealth(now)
	if !h.Durable.Available || h.Durable.Pending != 2 || h.Durable.Staged != 1 || h.Durable.Storage.Rows != 2 || h.OldestPendingAgeSeconds != 60 || h.Durable.LastPublishedMinute != 0 || !h.SnapshotStale {
		t.Fatalf("incorrect health: %+v", h)
	}
	raw, _ := json.Marshal(h)
	if strings.Contains(string(raw), "never-expose-payload") || strings.Contains(string(raw), "unfinished") {
		t.Fatal("health exposed attempt metadata")
	}
	metric.StartProduction(now.Add(-time.Minute))
	if err := model.DB.Exec("CREATE TRIGGER reject_health_publication BEFORE INSERT ON production_telemetry_minutes BEGIN SELECT RAISE(ABORT,'injected'); END").Error; err != nil {
		t.Fatal(err)
	}
	before := productionFlushHealth.snapshot().Errors
	if err := FlushProductionTelemetry(now); err == nil {
		t.Fatal("injected failure ignored")
	}
	h = ProductionTelemetryHealth(now)
	if !h.Flush.Failing || h.Flush.Errors != before+1 || h.Flush.LastFailure == 0 {
		t.Fatal(h.Flush)
	}
	model.DB.Exec("DROP TRIGGER reject_health_publication")
	if err := FlushProductionTelemetry(now); err != nil {
		t.Fatal(err)
	}
	h = ProductionTelemetryHealth(now.Add(6 * time.Second))
	if h.Flush.Failing || h.Durable.LastPublishedMinute != now.Add(-time.Minute).Unix() || h.Durable.LastPublishedAt == 0 {
		t.Fatalf("publication health did not recover: %+v", h)
	}
	if err := model.CloseDB(); err != nil {
		t.Fatal(err)
	}
	h = ProductionTelemetryHealth(now.Add(12 * time.Second))
	if h.Durable.Available {
		t.Fatal("closed storage claimed available")
	}
}

func TestProductionHealthSeparatesPoolTimeoutFromStorageFailure(t *testing.T) {
	resourceTestDB(t)
	t.Setenv("STARHUB_TELEMETRY_HEALTH_TIMEOUT", "20ms")
	pool, err := model.DB.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(1)
	connection, err := pool.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	h := durableProductionHealth(now)
	if h.Available || h.ErrorKind != "timeout" || h.Backend != "sqlite" || h.Pool.WaitCount == 0 || h.Pool.InUse != 1 {
		t.Fatalf("pool timeout was not identified: %+v", h)
	}
	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}
	h = durableProductionHealth(now.Add(6 * time.Second))
	if !h.Available || h.ErrorKind != "" {
		t.Fatal("health did not recover after pool release", h)
	}
}

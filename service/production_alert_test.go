package service

import (
	"slices"
	"starhub/metric"
	"starhub/model"
	"testing"
	"time"
)

func TestProductionAlertReasonsDistinguishIdleFromFailure(t *testing.T) {
	now := time.Now()
	h := ProductionTelemetryStatus{Collector: metric.ProductionCollectorHealth{StartedAt: now.Add(-time.Hour).Unix(), MaxBytes: 128 << 20}, Durable: ProductionDurableHealth{Available: true, LastPublishedMinute: now.Add(-time.Minute).Unix(), Storage: model.ProductionStorageState{MaxRows: 100000, MaxBytes: 128 << 20}}}
	if reasons := productionAlertReasons(h, now); len(reasons) != 0 {
		t.Fatal("healthy idle collector alerted", reasons)
	}
	h.Flush.Failing = true
	h.DeliveryEnabled = true
	h.Delivery.Failing = true
	h.OldestPendingAgeSeconds = 301
	h.SnapshotStale = true
	h.Durable.Storage.Rows = 90000
	reasons := productionAlertReasons(h, now)
	for _, want := range []string{"persistence_failed", "delivery_failed", "delivery_backlog", "routing_snapshot_stale", "storage_capacity"} {
		if !slices.Contains(reasons, want) {
			t.Fatal("missing alert", want, reasons)
		}
	}
}

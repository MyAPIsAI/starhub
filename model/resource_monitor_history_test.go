package model

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"starhub/config"
)

func monitoringTestSetup(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "monitoring.db")
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	if err := config.ConfigureSnapshotSource("", ""); err != nil {
		t.Fatal(err)
	}
	if err := config.ConfigureResourceMonitoring("http://127.0.0.1/events", "", "", "", "", ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = config.ConfigureSnapshotSource("", "")
		_ = config.ConfigureResourceMonitoring("", "", "", "", "", "")
		_ = CloseDB()
	})
	return path
}

func priceTestEvent(version int64, id string, p int) ResourcePriceEvent {
	e := ResourcePriceEvent{EventID: id, Version: version, Type: "resource.upsert", OccurredAt: "2026-11-06T00:00:00Z"}
	e.Resource.ResourceID, e.Resource.PriceTier = "resource-1", &p
	return e
}

func TestResourceSnapshotAgeUsesPublicationAndRecovers(t *testing.T) {
	monitoringTestSetup(t)
	now := time.Now().Truncate(time.Second)
	snapshot := resourceFixture()
	snapshot.Sequence = 300
	snapshot.PublishedAt = now.Add(-190 * time.Second).Format(time.RFC3339)
	if _, err := SaveResourceSnapshot(t.Context(), 0, snapshot); err != nil {
		t.Fatal(err)
	}
	view, err := ReadResourceSyncStatus(t.Context(), now)
	if err != nil || view.SnapshotStatus.Status != "expired" || *view.SnapshotStatus.AgeSeconds != 190 || view.SnapshotStatus.Sequence != 300 {
		t.Fatalf("expired snapshot: %+v %v", view, err)
	}
	if s := resourceSnapshotStatus(&snapshot, now.Add(-10*time.Second)); s.Status != "fresh" {
		t.Fatal("exactly 180 seconds must remain fresh")
	}
	snapshot.Sequence++
	snapshot.PublishedAt = now.Format(time.RFC3339)
	if _, err := SaveResourceSnapshot(t.Context(), 1, snapshot); err != nil {
		t.Fatal(err)
	}
	view, _ = ReadResourceSyncStatus(t.Context(), now)
	if view.SnapshotStatus.Status != "fresh" || *view.SnapshotStatus.AgeSeconds != 0 {
		t.Fatal("new snapshot did not recover")
	}
	if s := resourceSnapshotStatus(&snapshot, now.Add(-time.Second)); s.Status != "clock_skew" {
		t.Fatal("future publication marked healthy")
	}
	if s := resourceSnapshotStatus(nil, now); s.Status != "missing" || s.AgeSeconds != nil {
		t.Fatal("missing snapshot fabricated age")
	}
}

func TestResourcePriceEventsAtomicOrderingHistoryAndRestart(t *testing.T) {
	path := monitoringTestSetup(t)
	now := time.Now()
	snapshot := resourceFixture()
	p := 4
	snapshot.Instances[0].PriceTier = &p
	if _, err := SaveResourceSnapshot(t.Context(), 0, snapshot); err != nil {
		t.Fatal(err)
	}
	batch := ResourcePriceEventBatch{Events: []ResourcePriceEvent{priceTestEvent(1, "first", 6)}}
	for range 2 {
		if err := ApplyResourcePriceEvents(t.Context(), batch, now); err != nil {
			t.Fatal(err)
		}
	}
	history, err := ReadResourceMonitorHistory(t.Context(), 1, 1)
	if err != nil || history.MismatchTotal != 1 || history.Mismatches[0].SnapshotPrice != 4 || history.Mismatches[0].EventPrice != 6 {
		t.Fatalf("missing mismatch: %+v %v", history, err)
	}
	bad := ResourcePriceEventBatch{BaseVersion: 1, Events: []ResourcePriceEvent{priceTestEvent(2, "second", 8), priceTestEvent(4, "gap", 10)}}
	if err := ApplyResourcePriceEvents(t.Context(), bad, now); err == nil {
		t.Fatal("event gap accepted")
	}
	catalog, _ := ReadResourceCatalog(t.Context())
	if *catalog.Snapshot.Instances[0].PriceTier != 6 {
		t.Fatal("partial event batch leaked")
	}
	conflict := ResourcePriceEventBatch{Events: []ResourcePriceEvent{priceTestEvent(1, "first", 8)}}
	if err := ApplyResourcePriceEvents(t.Context(), conflict, now); !errors.Is(err, ErrResourceConflict) {
		t.Fatalf("conflicting retry accepted: %v", err)
	}
	batch = ResourcePriceEventBatch{BaseVersion: 1, Events: []ResourcePriceEvent{priceTestEvent(2, "second", 8)}}
	if err := ApplyResourcePriceEvents(t.Context(), batch, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	history, _ = ReadResourceMonitorHistory(t.Context(), 1, 1)
	if history.MismatchTotal != 2 || history.Mismatches[1].ResolvedAt == 0 {
		t.Fatal("mismatch history was overwritten")
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	catalog, _ = ReadResourceCatalog(t.Context())
	if *catalog.Snapshot.Instances[0].PriceTier != 8 {
		t.Fatal("event authority lost after restart")
	}
	var stored ResourceCatalogRecord
	DB.First(&stored, 1)
	var raw ResourceSnapshot
	_ = json.Unmarshal([]byte(stored.Snapshot), &raw)
	if *raw.Instances[0].PriceTier != 4 {
		t.Fatal("raw snapshot evidence overwritten")
	}
	p = 8
	snapshot.Sequence++
	if _, err := SaveResourceSnapshot(t.Context(), 1, snapshot); err != nil {
		t.Fatal(err)
	}
	history, _ = ReadResourceMonitorHistory(t.Context(), 1, 1)
	if history.MismatchTotal != 2 || history.Mismatches[0].ResolvedAt == 0 {
		t.Fatal("recovery erased history or left alert active")
	}
}

func TestResourcePriceBaselineAndDirectoryCatchup(t *testing.T) {
	monitoringTestSetup(t)
	now := time.Now()
	baseline := []ResourceDirectoryPrice{{ResourceID: "resource-1", PriceTier: 6}}
	batch := ResourcePriceEventBatch{BaseVersion: 100, Baseline: &baseline}
	if err := ApplyResourcePriceEvents(t.Context(), batch, now); err != nil {
		t.Fatal(err)
	}
	if err := ApplyResourcePriceEvents(t.Context(), batch, now); err != nil {
		t.Fatal(err)
	}
	snapshot := resourceFixture()
	p := 4
	snapshot.Instances[0].PriceTier = &p
	if err := DB.Create(&ResourceSyncState{ID: 1, Source: resourceSourceID(), Version: 101}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := SaveResourceSnapshot(t.Context(), 0, snapshot); err != nil {
		t.Fatal(err)
	}
	history, _ := ReadResourceMonitorHistory(t.Context(), 1, 1)
	if history.MismatchTotal != 0 {
		t.Fatal("lagging events created a false mismatch")
	}
	batch = ResourcePriceEventBatch{BaseVersion: 100, Events: []ResourcePriceEvent{priceTestEvent(101, "catchup", 6)}}
	if err := ApplyResourcePriceEvents(t.Context(), batch, now); err != nil {
		t.Fatal(err)
	}
	history, _ = ReadResourceMonitorHistory(t.Context(), 1, 1)
	if history.MismatchTotal != 1 {
		t.Fatal("caught-up events were not compared")
	}
}

func TestResourceIncidentDeliveryRetriesSameEventAfterRestart(t *testing.T) {
	path := monitoringTestSetup(t)
	var payloads []string
	reject := true
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if payload["event_id"] != r.Header.Get("Idempotency-Key") || payload["gateway_id"] != "gw-2" {
			t.Error("unstable incident identity")
		}
		raw, _ := json.Marshal(payload)
		payloads = append(payloads, string(raw))
		if reject {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(204)
	}))
	defer source.Close()
	if err := config.ConfigureResourceMonitoring("http://127.0.0.1/events", "", source.URL, "", "", ""); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	event := newResourceIncident("half_open_failed", "resource-1", "gw-2", now)
	if err := DB.Create(&event).Error; err != nil {
		t.Fatal(err)
	}
	if err := DeliverResourceIncidents(t.Context(), now); err != nil {
		t.Fatal(err)
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	reject = false
	if err := DeliverResourceIncidents(t.Context(), now.Add(6*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := DeliverResourceIncidents(t.Context(), now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(payloads) != 2 || payloads[0] != payloads[1] {
		t.Fatalf("delivery was duplicated or payload changed: %d", len(payloads))
	}
	history, _ := ReadResourceMonitorHistory(t.Context(), 1, 1)
	if history.IncidentTotal != 1 || history.Incidents[0].DeliveryStatus != "delivered" || history.Incidents[0].Attempts != 2 {
		t.Fatalf("delivery status lost: %+v", history)
	}
}

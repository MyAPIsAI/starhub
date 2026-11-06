package model

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"starhub/config"
)

func TestADMAC08ScheduledOutageRecoveryAndDurableHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resource-monitor.db")
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	now := time.Now().Truncate(time.Second)
	snapshot := resourceFixture()
	var failed atomic.Bool
	var calls atomic.Int32
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Connection", "close")
		calls.Add(1)
		if r.URL.Query().Get("full") != "true" || r.Header.Get("Authorization") != "Bearer private-fixture" {
			t.Error("full sync authentication missing")
		}
		if failed.Load() {
			connection, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = connection.Close()
			}
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"version": 12, "checksum": ResourceSnapshotChecksum(snapshot), "snapshot": snapshot})
	}))
	defer source.Close()
	if err := config.ConfigureSnapshotSource(source.URL, "private-fixture"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = config.ConfigureSnapshotSource("", "") })
	view, err := SyncResourcesOnce(t.Context(), now, 0)
	if err != nil || view.Version != 12 || view.Sequence != 1 || view.Status != "aligned" {
		t.Fatalf("initial: %+v %v", view, err)
	}
	if _, err := SyncResourcesOnce(t.Context(), now.Add(time.Minute), 0); !errors.Is(err, ErrResourceSyncBusy) {
		t.Fatalf("unscheduled run: %v", err)
	}
	failed.Store(true)
	for _, minute := range []int{5, 10} {
		if _, err := SyncResourcesOnce(t.Context(), now.Add(time.Duration(minute)*time.Minute), 0); err == nil {
			t.Fatal("outage succeeded")
		}
	}
	view, err = ReadResourceSyncStatus(t.Context(), now.Add(10*time.Minute))
	if err != nil || view.Status != "alert" || view.FailedCycles != 2 || len(view.Failures) != 2 || view.LastAlignedAt != now.Unix() {
		t.Fatalf("outage: %+v %v", view, err)
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	view, err = ReadResourceSyncStatus(t.Context(), now.Add(10*time.Minute))
	if err != nil || view.Status != "alert" || len(view.Failures) != 2 {
		t.Fatalf("restart lost failure history: %+v %v", view, err)
	}
	failed.Store(false)
	view, err = SyncResourcesOnce(t.Context(), now.Add(15*time.Minute), 0)
	if err != nil || view.Status != "aligned" || view.FailedCycles != 0 || view.Revision != 1 || len(view.Failures) != 2 || *view.VersionGap != 0 {
		t.Fatalf("recovery: %+v %v", view, err)
	}
	raw, _ := json.Marshal(view)
	if strings.Contains(string(raw), "private-fixture") || strings.Contains(string(raw), source.URL) {
		t.Fatal("source credential leaked")
	}
	if calls.Load() != 4 {
		t.Fatalf("poll count %d", calls.Load())
	}
	// Snapshot health advances independently of the unchanged directory version.
	snapshot.Sequence++
	snapshot.Instances[0].Trend = "degraded"
	view, err = SyncResourcesOnce(t.Context(), now.Add(20*time.Minute), 0)
	if err != nil || view.Version != 12 || view.Sequence != 2 {
		t.Fatalf("independent snapshot: %+v %v", view, err)
	}
}

func TestResourceSyncAtomicAuditAndLease(t *testing.T) {
	newTestDB(t)
	now := time.Now().Truncate(time.Second)
	snapshot := resourceFixture()
	entered, release := make(chan struct{}, 1), make(chan struct{})
	var block atomic.Bool
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if block.Load() {
			entered <- struct{}{}
			<-release
		}
		_ = json.NewEncoder(w).Encode(snapshot)
	}))
	defer source.Close()
	_ = config.ConfigureSnapshotSource(source.URL, "")
	t.Cleanup(func() { _ = config.ConfigureSnapshotSource("", "") })
	if _, err := SyncResourcesOnce(t.Context(), now, 1); err != nil {
		t.Fatal(err)
	}
	block.Store(true)
	done := make(chan error, 1)
	go func() { _, err := SyncResourcesOnce(t.Context(), now.Add(time.Second), 1); done <- err }()
	<-entered
	_, err := SyncResourcesOnce(t.Context(), now.Add(2*time.Second), 1)
	close(release)
	if !errors.Is(err, ErrResourceSyncBusy) {
		t.Fatalf("parallel writer allowed: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	block.Store(false)
	snapshot.Sequence = 2
	snapshot.Instances[0].CanonicalModel = "changed"
	if err := DB.Exec("CREATE TRIGGER fail_sync_audit BEFORE INSERT ON audit_logs BEGIN SELECT RAISE(ABORT, 'fixture'); END").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := SyncResourcesOnce(t.Context(), now.Add(3*time.Second), 1); err == nil {
		t.Fatal("audit failure committed")
	}
	view, err := ReadResourceSyncStatus(t.Context(), now.Add(3*time.Second))
	if err != nil || view.Version != 1 || view.Sequence != 1 || view.Revision != 1 {
		t.Fatalf("partial publication: %+v %v", view, err)
	}
}

func TestADMAC10IndependentGatewayStatesAndFiveMinuteWindow(t *testing.T) {
	newTestDB(t)
	now := time.Now().Truncate(time.Second)
	snapshot := resourceFixture()
	p := 4
	snapshot.Instances[0].PriceTier = &p
	snapshot.Instances[0].QualityLevel = "S0"
	if _, err := SaveResourceSnapshot(t.Context(), 0, snapshot); err != nil {
		t.Fatal(err)
	}
	r := snapshot.Instances[0]
	a, b := NewResourceRuntime("gateway-a"), NewResourceRuntime("gateway-b")
	a.Record(r, 1, now.Add(-5*time.Minute), "success", 0)
	for range 5 {
		a.Record(r, 1, now, "failure", 0)
	}
	b.Record(r, 1, now, "success", 0)
	for _, runtime := range []*ResourceRuntime{a, b} {
		if err := runtime.Publish(t.Context(), now); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := ReadAdminResourceStatus(t.Context(), now)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows: %+v %v", rows, err)
	}
	row := rows[0]
	if row.Calls != 6 || row.Successes != 1 || *row.SuccessRate != 1.0/6 || row.Gateways[0].State != "open" || row.Gateways[1].State != "normal" {
		t.Fatalf("wrong aggregation: %+v", row)
	}
	// Reads and probes must never count as customer attempts.
	a.FinishProbe(r, now.Add(30*time.Second), true)
	if err := a.Publish(t.Context(), now.Add(30*time.Second)); err != nil {
		t.Fatal(err)
	}
	rows, err = ReadAdminResourceStatus(t.Context(), now.Add(30*time.Second))
	if err != nil || rows[0].Calls != 6 || rows[0].Gateways[0].State != "normal" || rows[0].Gateways[1].State != "unknown" {
		t.Fatalf("freshness/probes: %+v %v", rows, err)
	}
	rows, err = ReadAdminResourceStatus(t.Context(), now.Add(5*time.Minute))
	if err != nil || rows[0].Calls != 0 || rows[0].SuccessRate != nil {
		t.Fatalf("time window did not expire: %+v %v", rows, err)
	}
	if NewResourceRuntime("restart").State(r, now) != "normal" {
		t.Fatal("restart restored circuit")
	}
}

func TestResourceLocalHealthIgnoresClientFailuresAndRecovers(t *testing.T) {
	r := resourceFixture().Instances[0]
	r.QualityLevel = "S0"
	now := time.Now()
	runtime := NewResourceRuntime("health")
	for range 4 {
		runtime.Record(r, 1, now, "failure", 0)
	}
	for range 5 {
		runtime.Record(r, 1, now, "ignored", 0)
	}
	if runtime.State(r, now) != "normal" {
		t.Fatal("ignored outcomes opened circuit")
	}
	runtime.Record(r, 1, now, "failure", 0)
	if runtime.State(r, now) != "open" || len(runtime.DueProbes(now.Add(29*time.Second))) != 0 || len(runtime.DueProbes(now.Add(30*time.Second))) != 1 {
		t.Fatal("circuit/probe timing")
	}
	runtime.FinishProbe(r, now.Add(30*time.Second), true)
	for range 3 {
		runtime.Record(r, 1, now, "success", 1200)
	}
	if runtime.State(r, now) != "degraded" {
		t.Fatal("EWMA did not deprioritize")
	}
	if runtime.State(r, now.Add(5*time.Minute)) != "normal" {
		t.Fatal("deprioritization did not expire")
	}
}

package model

import (
	"testing"
	"time"
)

func TestResourcePriceBatchRetainsEveryConflict(t *testing.T) {
	for _, test := range []struct {
		name      string
		lastPrice int
		want      int64
	}{
		{"two_conflicts", 8, 2},
		{"conflict_then_recovery", 4, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := monitoringTestSetup(t)
			now := time.Now().Truncate(time.Second)
			snapshot := resourceFixture()
			p := 4
			snapshot.Instances[0].PriceTier = &p
			if _, err := SaveResourceSnapshot(t.Context(), 0, snapshot); err != nil {
				t.Fatal(err)
			}
			if err := DB.Create(&ResourceSyncState{ID: 1, Source: resourceSourceID(), NextAttemptAt: now.Add(time.Minute).Unix()}).Error; err != nil {
				t.Fatal(err)
			}
			batch := ResourcePriceEventBatch{Events: []ResourcePriceEvent{
				priceTestEvent(1, "first", 6), priceTestEvent(2, "second", test.lastPrice),
			}}
			for range 2 {
				if err := ApplyResourcePriceEvents(t.Context(), batch, now); err != nil {
					t.Fatal(err)
				}
			}
			if err := CloseDB(); err != nil {
				t.Fatal(err)
			}
			if err := InitDB(path); err != nil {
				t.Fatal(err)
			}
			history, err := ReadResourceMonitorHistory(t.Context(), 1, 1)
			if err != nil {
				t.Fatal(err)
			}
			if history.MismatchTotal != test.want {
				t.Fatalf("got %d conflicts, want %d", history.MismatchTotal, test.want)
			}
			first := history.Mismatches[len(history.Mismatches)-1]
			if first.ResourceID != "resource-1" || first.SnapshotPrice != 4 || first.EventPrice != 6 || first.EventVersion != 1 || first.DetectedAt != now.Unix() || first.ResolvedAt != now.Unix() {
				t.Fatalf("first conflict lost its evidence or resolution: %+v", first)
			}
			if test.want == 2 {
				last := history.Mismatches[0]
				if last.EventVersion != 2 || last.EventPrice != 8 || last.ResolvedAt != 0 {
					t.Fatalf("latest conflict is not active or ordered first: %+v", last)
				}
			}
			catalog, err := ReadResourceCatalog(t.Context())
			if err != nil || *catalog.Snapshot.Instances[0].PriceTier != test.lastPrice {
				t.Fatalf("final event price not applied: %v", err)
			}
			var state ResourceSyncState
			if err := DB.First(&state, 1).Error; err != nil || state.NextAttemptAt != 0 {
				t.Fatalf("alignment not requested: %+v %v", state, err)
			}
		})
	}
}

func TestResourcePriceBatchRollbackIncludesHistoryAndAlignment(t *testing.T) {
	monitoringTestSetup(t)
	now := time.Now().Truncate(time.Second)
	snapshot := resourceFixture()
	p := 4
	snapshot.Instances[0].PriceTier = &p
	if _, err := SaveResourceSnapshot(t.Context(), 0, snapshot); err != nil {
		t.Fatal(err)
	}
	next := now.Add(time.Minute).Unix()
	if err := DB.Create(&ResourceSyncState{ID: 1, Source: resourceSourceID(), NextAttemptAt: next}).Error; err != nil {
		t.Fatal(err)
	}
	batch := ResourcePriceEventBatch{Events: []ResourcePriceEvent{
		priceTestEvent(1, "first", 6), priceTestEvent(2, "second", 8), priceTestEvent(4, "gap", 10),
	}}
	if err := ApplyResourcePriceEvents(t.Context(), batch, now); err == nil {
		t.Fatal("version gap accepted")
	}
	history, err := ReadResourceMonitorHistory(t.Context(), 1, 1)
	if err != nil || history.MismatchTotal != 0 || len(history.Feeds) != 0 {
		t.Fatalf("failed batch leaked history or cursor: %+v %v", history, err)
	}
	var count int64
	if err := DB.Model(&ResourceObservedEvent{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("failed batch leaked event deduplication state: %d %v", count, err)
	}
	catalog, err := ReadResourceCatalog(t.Context())
	if err != nil || *catalog.Snapshot.Instances[0].PriceTier != 4 {
		t.Fatalf("failed batch changed price: %v", err)
	}
	var state ResourceSyncState
	if err := DB.First(&state, 1).Error; err != nil || state.NextAttemptAt != next {
		t.Fatalf("failed batch requested alignment: %+v %v", state, err)
	}
}

func TestResourcePriceBatchCatchupSkipsObsoleteConflicts(t *testing.T) {
	monitoringTestSetup(t)
	now := time.Now()
	snapshot := resourceFixture()
	p := 4
	snapshot.Instances[0].PriceTier = &p
	if err := DB.Create(&ResourceSyncState{ID: 1, Source: resourceSourceID(), Version: 102}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := SaveResourceSnapshot(t.Context(), 0, snapshot); err != nil {
		t.Fatal(err)
	}
	baseline := []ResourceDirectoryPrice{{ResourceID: "resource-1", PriceTier: 6}}
	batch := ResourcePriceEventBatch{BaseVersion: 100, Baseline: &baseline, Events: []ResourcePriceEvent{
		priceTestEvent(101, "obsolete", 8), priceTestEvent(102, "current", 4), priceTestEvent(103, "new-conflict", 6),
	}}
	if err := ApplyResourcePriceEvents(t.Context(), batch, now); err != nil {
		t.Fatal(err)
	}
	history, err := ReadResourceMonitorHistory(t.Context(), 1, 1)
	if err != nil || history.MismatchTotal != 1 || history.Mismatches[0].EventVersion != 103 {
		t.Fatalf("obsolete events produced false conflicts: %+v %v", history, err)
	}
}

func TestResourcePriceBatchRetainsBootstrapConflict(t *testing.T) {
	monitoringTestSetup(t)
	now := time.Now()
	snapshot := resourceFixture()
	p := 4
	snapshot.Instances[0].PriceTier = &p
	if _, err := SaveResourceSnapshot(t.Context(), 0, snapshot); err != nil {
		t.Fatal(err)
	}
	baseline := []ResourceDirectoryPrice{{ResourceID: "resource-1", PriceTier: 6}}
	batch := ResourcePriceEventBatch{BaseVersion: 100, Baseline: &baseline, Events: []ResourcePriceEvent{priceTestEvent(101, "recovered", 4)}}
	if err := ApplyResourcePriceEvents(t.Context(), batch, now); err != nil {
		t.Fatal(err)
	}
	history, err := ReadResourceMonitorHistory(t.Context(), 1, 1)
	if err != nil || history.MismatchTotal != 1 || history.Mismatches[0].EventVersion != 100 || history.Mismatches[0].ResolvedAt == 0 {
		t.Fatalf("bootstrap conflict disappeared on recovery: %+v %v", history, err)
	}
}

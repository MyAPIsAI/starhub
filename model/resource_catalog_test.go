package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	"gorm.io/gorm"
)

func resourceFixture() ResourceSnapshot {
	return ResourceSnapshot{ID: "inventory-1", Sequence: 1, PublishedAt: "2026-10-28T00:00:00Z", Source: "test inventory", Channels: []ResourceChannel{{ID: "supplier", Name: "Supplier", Enabled: true}}, Instances: []ResourceInstance{{ID: "resource-1", ChannelID: "supplier", UpstreamModel: "supplier-text", CanonicalModel: "standard-text", Protocols: []string{"OpenAI"}, Capabilities: []string{"text"}, ContextLength: 128000, Region: "", Status: "available", LatencyMS: 0}}}
}

func resourcePlacementFixture() ResourcePlacement {
	model := "standard-text"
	return ResourcePlacement{ModelID: &model, GroupIDs: []string{"default"}, Enabled: true}
}

func TestResourceCatalogPersistenceAndNoAutomaticAccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resources.db")
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	ctx := context.Background()
	empty, err := ReadResourceCatalog(ctx)
	if err != nil || empty.Revision != 0 || empty.Snapshot != nil || len(empty.Placements) != 0 || empty.AffectsGatewayRouting {
		t.Fatalf("initial state: %+v %v", empty, err)
	}
	state, err := SaveResourceSnapshot(ctx, 0, resourceFixture())
	if err != nil {
		t.Fatal(err)
	}
	if state.Revision != 1 || state.Placements["resource-1"].Enabled || state.Placements["resource-1"].ModelID != nil || len(state.Placements["resource-1"].GroupIDs) != 0 || state.Snapshot.ReceivedAt == "" {
		t.Fatalf("import granted access or lost metadata: %+v", state)
	}
	state, err = SaveResourcePlacement(ctx, 1, "resource-1", resourcePlacementFixture())
	if err != nil {
		t.Fatal(err)
	}
	if !state.Placements["resource-1"].Enabled || state.Revision != 2 {
		t.Fatalf("placement not saved: %+v", state)
	}
	for _, entity := range []any{&Channel{}, &Ability{}} {
		var count int64
		if err := DB.Model(entity).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("inventory changed gateway routing: %d %v", count, err)
		}
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	reopened, err := ReadResourceCatalog(ctx)
	if err != nil || !reflect.DeepEqual(state, reopened) {
		t.Fatalf("restart lost state: %+v %v", reopened, err)
	}
	if _, err := SaveResourcePlacement(ctx, 1, "resource-1", ResourcePlacement{GroupIDs: []string{}}); !errors.Is(err, ErrResourceConflict) {
		t.Fatalf("stale editor not rejected: %v", err)
	}
	reopened, _ = ReadResourceCatalog(ctx)
	if !reflect.DeepEqual(state, reopened) {
		t.Fatal("stale request changed data")
	}
}

func TestResourceImportChangesCannotExpandPlacements(t *testing.T) {
	for _, change := range []string{"upstream", "canonical", "channel", "retired", "removed-readded"} {
		t.Run(change, func(t *testing.T) {
			newTestDB(t)
			ctx := context.Background()
			snapshot := resourceFixture()
			if _, err := SaveResourceSnapshot(ctx, 0, snapshot); err != nil {
				t.Fatal(err)
			}
			if _, err := SaveResourcePlacement(ctx, 1, "resource-1", resourcePlacementFixture()); err != nil {
				t.Fatal(err)
			}
			snapshot.Sequence = 2
			switch change {
			case "upstream":
				snapshot.Instances[0].UpstreamModel = "other-upstream"
			case "canonical":
				snapshot.Instances[0].CanonicalModel = "other-standard"
			case "channel":
				snapshot.Channels = append(snapshot.Channels, ResourceChannel{ID: "other", Name: "Other", Enabled: true})
				snapshot.Instances[0].ChannelID = "other"
			case "retired":
				snapshot.Instances[0].Status = "retired"
			case "removed-readded":
				snapshot.Instances = []ResourceInstance{}
			}
			state, err := SaveResourceSnapshot(ctx, 2, snapshot)
			if err != nil {
				t.Fatal(err)
			}
			if change == "removed-readded" {
				if len(state.Placements) != 0 {
					t.Fatal("removed inventory retained placement")
				}
				snapshot = resourceFixture()
				snapshot.Sequence = 3
				state, err = SaveResourceSnapshot(ctx, 3, snapshot)
				if err != nil {
					t.Fatal(err)
				}
			}
			if state.Placements["resource-1"].Enabled {
				t.Fatal("changed or readded resource retained authorization")
			}
		})
	}
}

func TestResourceCatalogAtomicRollbackAndSingleWinner(t *testing.T) {
	newTestDB(t)
	ctx := context.Background()
	const callback = "fail_resource_snapshot"
	if err := DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "resource_catalog_records" {
			if values, ok := tx.Statement.Dest.(map[string]any); ok && values["snapshot"] != nil {
				tx.AddError(errors.New("injected inventory failure"))
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveResourceSnapshot(ctx, 0, resourceFixture()); err == nil {
		t.Fatal("injected failure did not fail")
	}
	DB.Callback().Update().Remove(callback)
	state, err := ReadResourceCatalog(ctx)
	if err != nil || state.Revision != 0 || state.Snapshot != nil || len(state.Placements) != 0 {
		t.Fatalf("partial transaction: %+v %v", state, err)
	}
	var winners atomic.Int64
	var wg sync.WaitGroup
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			snapshot := resourceFixture()
			snapshot.Source = fmt.Sprintf("writer-%d", i)
			_, err := SaveResourceSnapshot(ctx, 0, snapshot)
			if err == nil {
				winners.Add(1)
			} else if !errors.Is(err, ErrResourceConflict) {
				failures <- err
			}
		}(i)
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	if winners.Load() != 1 {
		t.Fatalf("concurrent winners=%d", winners.Load())
	}
	state, _ = ReadResourceCatalog(ctx)
	if state.Revision != 1 || len(state.Placements) != 1 {
		t.Fatalf("unexpected final state: %+v", state)
	}
}

func TestResourceCatalogValidationAndCancellation(t *testing.T) {
	newTestDB(t)
	ctx := context.Background()
	for _, test := range []struct {
		name   string
		mutate func(*ResourceSnapshot)
	}{
		{"virtual upstream", func(s *ResourceSnapshot) { s.Instances[0].UpstreamModel = "myapis-auto" }},
		{"virtual canonical", func(s *ResourceSnapshot) { s.Instances[0].CanonicalModel = "veyra-auto" }},
		{"duplicate ID", func(s *ResourceSnapshot) { s.Instances = append(s.Instances, s.Instances[0]) }},
		{"missing channel", func(s *ResourceSnapshot) { s.Instances[0].ChannelID = "missing" }},
		{"invalid protocol", func(s *ResourceSnapshot) { s.Instances[0].Protocols = []string{"unknown"} }},
		{"duplicate capability", func(s *ResourceSnapshot) { s.Instances[0].Capabilities = []string{"text", "text"} }},
		{"unknown status", func(s *ResourceSnapshot) { s.Instances[0].Status = "healthy" }},
		{"negative latency", func(s *ResourceSnapshot) { s.Instances[0].LatencyMS = -1 }},
		{"unsafe context", func(s *ResourceSnapshot) { s.Instances[0].ContextLength = 1 << 53 }},
		{"bad publication", func(s *ResourceSnapshot) { s.PublishedAt = "yesterday" }},
		{"null list", func(s *ResourceSnapshot) { s.Instances = nil }},
		{"control character", func(s *ResourceSnapshot) { s.Instances[0].Region = "region\nlabel" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			snapshot := resourceFixture()
			test.mutate(&snapshot)
			if _, err := SaveResourceSnapshot(ctx, 0, snapshot); !errors.Is(err, ErrInvalidResource) {
				t.Fatalf("invalid snapshot accepted: %v", err)
			}
		})
	}
	if _, err := SaveResourceSnapshot(ctx, 0, resourceFixture()); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveResourceSnapshot(ctx, 1, resourceFixture()); !errors.Is(err, ErrResourceConflict) {
		t.Fatalf("replayed sequence accepted: %v", err)
	}
	for _, placement := range []ResourcePlacement{{GroupIDs: []string{}, Enabled: true}, {GroupIDs: []string{"default", "default"}}, {GroupIDs: nil}, {ModelID: func() *string { v := "myapis-auto"; return &v }(), GroupIDs: []string{}}} {
		if _, err := SaveResourcePlacement(ctx, 1, "resource-1", placement); !errors.Is(err, ErrInvalidResource) {
			t.Fatalf("invalid placement accepted: %v", err)
		}
	}
	if _, err := SaveResourcePlacement(ctx, 1, "missing", resourcePlacementFixture()); !errors.Is(err, ErrInvalidResource) {
		t.Fatalf("missing resource accepted: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := SaveResourcePlacement(cancelled, 1, "resource-1", resourcePlacementFixture()); err == nil {
		t.Fatal("cancelled write accepted")
	}
	state, _ := ReadResourceCatalog(ctx)
	if state.Revision != 1 {
		t.Fatal("invalid or cancelled writes advanced revision")
	}
	// A channel outage is inventory state, and cannot be newly enabled through a placement edit.
	snapshot := resourceFixture()
	snapshot.Sequence = 2
	snapshot.Channels[0].Enabled = false
	if _, err := SaveResourceSnapshot(ctx, 1, snapshot); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveResourcePlacement(ctx, 2, "resource-1", resourcePlacementFixture()); !errors.Is(err, ErrInvalidResource) {
		t.Fatalf("unavailable resource enabled: %v", err)
	}
	encoded, _ := json.Marshal(state)
	if !json.Valid(encoded) {
		t.Fatal("catalog JSON invalid")
	}
}

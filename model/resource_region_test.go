package model

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

func TestResourceRegionsPersistWithoutInventingDeployment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "regions.db")
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	ctx := context.Background()
	snapshot := resourceFixture()
	snapshot.Instances[0].Region = "observed"
	state, err := SaveResourceSnapshot(ctx, 0, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Regions) != 0 {
		t.Fatal("import created a region definition")
	}
	state, err = SaveResourceRegion(ctx, 1, ResourceRegion{ID: "observed", Name: "Primary region", Description: "Inventory description", UpdatedAt: 1})
	if err != nil {
		t.Fatal(err)
	}
	if state.Regions["observed"].UpdatedAt <= 1 || state.AffectsGatewayRouting || state.Snapshot.Instances[0].Region != "observed" {
		t.Fatalf("invalid region scope: %+v", state)
	}
	if _, err := SaveResourceRegion(ctx, 1, ResourceRegion{ID: "observed", Name: "Stale name"}); !errors.Is(err, ErrResourceConflict) {
		t.Fatalf("stale rename: %v", err)
	}
	if _, err := DeleteResourceRegion(ctx, 2, "observed"); !errors.Is(err, ErrResourceRegionInUse) {
		t.Fatalf("referenced region deleted: %v", err)
	}
	after, _ := ReadResourceCatalog(ctx)
	if !reflect.DeepEqual(state, after) {
		t.Fatal("rejected deletion advanced revision")
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	after, err = ReadResourceCatalog(ctx)
	if err != nil || !reflect.DeepEqual(state, after) {
		t.Fatalf("region lost after restart: %+v %v", after, err)
	}
	state, err = SaveResourceRegion(ctx, 2, ResourceRegion{ID: "observed", Name: "Renamed display"})
	if err != nil {
		t.Fatal(err)
	}
	if state.Snapshot.Instances[0].Region != "observed" || state.Placements["resource-1"].Enabled {
		t.Fatal("display edit changed inventory identity or placement")
	}
	snapshot.Sequence = 2
	snapshot.Instances[0].Region = ""
	if _, err := SaveResourceSnapshot(ctx, 3, snapshot); err != nil {
		t.Fatal(err)
	}
	state, err = DeleteResourceRegion(ctx, 4, "observed")
	if err != nil || len(state.Regions) != 0 || state.Revision != 5 {
		t.Fatalf("unused delete: %+v %v", state, err)
	}
}

func TestResourceRegionValidationAndSharedRevision(t *testing.T) {
	newTestDB(t)
	ctx := context.Background()
	for _, region := range []ResourceRegion{{ID: "", Name: "Missing ID"}, {ID: "r", Name: ""}, {ID: " r", Name: "Bad ID"}, {ID: "r", Name: "Bad\nname"}} {
		if _, err := SaveResourceRegion(ctx, 0, region); !errors.Is(err, ErrInvalidResource) {
			t.Fatalf("invalid region accepted: %v", err)
		}
	}
	if _, err := DeleteResourceRegion(ctx, 0, "missing"); !errors.Is(err, ErrInvalidResource) {
		t.Fatalf("missing delete: %v", err)
	}
	var wg sync.WaitGroup
	outcomes := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, err := SaveResourceRegion(ctx, 0, ResourceRegion{ID: "r", Name: "Region"})
		outcomes <- err
	}()
	go func() { defer wg.Done(); _, err := SaveResourceSnapshot(ctx, 0, resourceFixture()); outcomes <- err }()
	wg.Wait()
	close(outcomes)
	success, conflict := 0, 0
	for err := range outcomes {
		if err == nil {
			success++
		} else if errors.Is(err, ErrResourceConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("inventory and region edits both won: %d %d", success, conflict)
	}
}

func TestResourceRegionCapacityStillAllowsExistingEdits(t *testing.T) {
	newTestDB(t)
	rows := make([]ResourceRegion, 1000)
	for i := range rows {
		rows[i] = ResourceRegion{ID: fmt.Sprintf("region-%d", i), Name: "Existing region"}
	}
	if err := DB.CreateInBatches(rows, 100).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := SaveResourceRegion(context.Background(), 0, ResourceRegion{ID: "extra", Name: "Extra"}); !errors.Is(err, ErrInvalidResource) {
		t.Fatalf("unbounded region growth accepted: %v", err)
	}
	state, err := SaveResourceRegion(context.Background(), 0, ResourceRegion{ID: "region-0", Name: "Updated"})
	if err != nil || len(state.Regions) != 1000 || state.Regions["region-0"].Name != "Updated" {
		t.Fatalf("existing edit at capacity: %v", err)
	}
}

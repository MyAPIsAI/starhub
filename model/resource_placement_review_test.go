package model

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"gorm.io/gorm"
)

func TestResourceIdentityReviewAndArchivePersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.db")
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	ctx := context.Background()
	snapshot := resourceFixture()
	state, err := SaveResourceSnapshot(ctx, 0, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	state, err = SaveResourcePlacement(ctx, state.Revision, "resource-1", resourcePlacementFixture())
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Sequence++
	snapshot.Instances[0].UpstreamModel = "changed"
	state, err = SaveResourceSnapshot(ctx, state.Revision, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if state.Placements["resource-1"].Enabled || state.PlacementReviews["resource-1"].Before.UpstreamModel != "supplier-text" {
		t.Fatal("missing identity hold")
	}
	before := state
	if _, err = SaveResourcePlacement(ctx, state.Revision, "resource-1", resourcePlacementFixture()); !errors.Is(err, ErrInvalidResource) {
		t.Fatal("unconfirmed identity enabled", err)
	}
	state, _ = ReadResourceCatalog(ctx)
	if !reflect.DeepEqual(before, state) {
		t.Fatal("rejection changed state")
	}
	// Returning to the original identity must still require human confirmation.
	snapshot.Sequence++
	snapshot.Instances[0].UpstreamModel = "supplier-text"
	state, err = SaveResourceSnapshot(ctx, state.Revision, snapshot)
	if err != nil || len(state.PlacementReviews) != 1 {
		t.Fatal("source rollback cleared review", err)
	}
	if err = CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err = InitDB(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := ReadResourceCatalog(ctx)
	if err != nil || !reflect.DeepEqual(state, loaded) {
		t.Fatal("restart lost review", err)
	}
	confirmed := resourcePlacementFixture()
	confirmed.ConfirmIdentity = true
	state, err = SaveResourcePlacement(ctx, state.Revision, "resource-1", confirmed)
	if err != nil || len(state.PlacementReviews) != 0 || !state.Placements["resource-1"].Enabled {
		t.Fatal("confirmation failed", err)
	}
	snapshot.Sequence++
	snapshot.Instances = []ResourceInstance{}
	state, err = SaveResourceSnapshot(ctx, state.Revision, snapshot)
	if err != nil || len(state.Placements) != 0 || len(state.RemovedPlacements) != 1 {
		t.Fatal("removal did not archive", err)
	}
	archive := state.RemovedPlacements[0]
	if archive.ModelID == nil || *archive.ModelID != "standard-text" || len(archive.Groups) != 1 {
		t.Fatal("archive lost mapping")
	}
	snapshot = resourceFixture()
	snapshot.Sequence = 5
	state, err = SaveResourceSnapshot(ctx, state.Revision, snapshot)
	if err != nil || state.Placements["resource-1"].ModelID != nil || state.Placements["resource-1"].Enabled || len(state.RemovedPlacements) != 1 {
		t.Fatal("reintroduced ID inherited authorization", err)
	}
	if err = CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err = InitDB(path); err != nil {
		t.Fatal(err)
	}
	loaded, err = ReadResourceCatalog(ctx)
	if err != nil || !reflect.DeepEqual(state, loaded) {
		t.Fatal("restart lost archive", err)
	}
	state, err = ClearRemovedResourcePlacement(ctx, state.Revision, archive.ID)
	if err != nil || len(state.RemovedPlacements) != 0 || len(state.Placements) != 1 {
		t.Fatal("cleanup affected current resource", err)
	}
}

func TestResourceArchiveRollbackRetainsPublishedInventory(t *testing.T) {
	_, state := resourceGatewayInventoryFixture(t)
	before := state
	const callback = "reject_resource_archive"
	err := DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "resource_placement_archives" {
			tx.AddError(errors.New("injected archive failure"))
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer DB.Callback().Create().Remove(callback)
	snapshot := *state.Snapshot
	snapshot.Sequence++
	snapshot.Instances = []ResourceInstance{}
	if _, err = SaveResourceSnapshot(context.Background(), state.Revision, snapshot); err == nil {
		t.Fatal("archive failure ignored")
	}
	after, err := ReadResourceCatalog(context.Background())
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("partial archive transaction", err)
	}
	var count int64
	DB.Model(&ResourceGatewayRoute{}).Count(&count)
	if count != 2 {
		t.Fatal("failed import revoked published routes")
	}
}

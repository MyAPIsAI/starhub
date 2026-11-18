package model

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
)

func TestChannelRegistersMissingModelsWithoutReplacingMetadataOrPrices(t *testing.T) {
	groupConfigurationDB(t)
	ctx := context.Background()
	price := 0.25
	existing := &ModelMeta{Model: "existing", Description: "Keep this", Tags: "image", OfficialPerRequest: &price}
	if err := UpsertModelMeta(existing); err != nil {
		t.Fatal(err)
	}
	options := groupConfigurationSnapshot(t)
	ch := &Channel{Models: " existing, New-Version-1, New-Version-1, ,ep-Exact ", Group: "default", Status: 1}
	if err := InsertChannel(ch); err != nil {
		t.Fatal(err)
	}
	if ch.Models != "existing,New-Version-1,ep-Exact" {
		t.Fatalf("model IDs were not preserved and deduplicated: %q", ch.Models)
	}
	catalog, err := ReadModelMetadataCatalog(ctx)
	if err != nil || catalog.Revision != 2 || len(catalog.Models) != 3 {
		t.Fatalf("catalog: %+v %v", catalog, err)
	}
	stored, _, err := GetModelMeta(existing.Model)
	if err != nil || !reflect.DeepEqual(stored, existing) {
		t.Fatalf("existing metadata changed: %+v %v", stored, err)
	}
	for _, name := range []string{"New-Version-1", "ep-Exact"} {
		meta, found, err := GetModelMeta(name)
		if err != nil || !found || meta.CreatedAt <= 0 {
			t.Fatalf("missing model %q: %+v %v", name, meta, err)
		}
		want := &ModelMeta{Model: name, AutoRegistered: true, CreatedAt: meta.CreatedAt}
		if !reflect.DeepEqual(meta, want) {
			t.Fatalf("invented model properties or prices: %+v", meta)
		}
	}
	if _, err := UpsertModelMetaIfRevision(ctx, existing, 1); !errors.Is(err, ErrModelMetadataConflict) {
		t.Fatalf("channel creation did not invalidate the old editor: %v", err)
	}
	if err := UpdateChannel(ch); err != nil {
		t.Fatal(err)
	}
	unchanged, err := ReadModelMetadataCatalog(ctx)
	if err != nil || !reflect.DeepEqual(catalog, unchanged) {
		t.Fatal("unchanged model set advanced the catalog revision")
	}
	ch.Models += ",another-model"
	if err := UpdateChannel(ch); err != nil {
		t.Fatal(err)
	}
	updated, err := ReadModelMetadataCatalog(ctx)
	if err != nil || updated.Revision != 3 || len(updated.Models) != 4 {
		t.Fatalf("edit did not register the added model: %+v %v", updated, err)
	}
	if err := DeleteChannel(ch.ID); err != nil {
		t.Fatal(err)
	}
	afterDelete, err := ReadModelMetadataCatalog(ctx)
	if err != nil || !reflect.DeepEqual(updated, afterDelete) || groupConfigurationSnapshot(t) != options {
		t.Fatal("channel changes deleted metadata or changed billing configuration")
	}
}

func TestChannelMetadataFailureRollsBackChannelAbilitiesAndRevisions(t *testing.T) {
	newTestDB(t)
	ch := &Channel{Models: "original", Group: "default", Status: 1}
	if err := InsertChannel(ch); err != nil {
		t.Fatal(err)
	}
	before, err := ReadModelMetadataCatalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	channelVersion := channelRevision(t)
	if err := DB.Exec(`CREATE TRIGGER reject_channel_metadata BEFORE UPDATE ON model_metadata_states WHEN NEW.revision > OLD.revision BEGIN SELECT RAISE(ABORT, 'synthetic metadata failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	for _, create := range []bool{true, false} {
		candidate := &Channel{Models: "must-rollback", Group: "default", Status: 1}
		if create {
			err = InsertChannel(candidate)
		} else {
			candidate.ID = ch.ID
			err = UpdateChannel(candidate)
		}
		if err == nil {
			t.Fatal("metadata failure did not abort the channel transaction")
		}
		after, readErr := ReadModelMetadataCatalog(context.Background())
		if readErr != nil || !reflect.DeepEqual(before, after) || channelRevision(t) != channelVersion {
			t.Fatal("failed channel save changed catalog or revisions")
		}
		var channels, abilities int64
		DB.Model(&Channel{}).Count(&channels)
		DB.Model(&Ability{}).Count(&abilities)
		stored, readErr := GetChannelByID(ch.ID)
		if readErr != nil || stored.Models != "original" || channels != 1 || abilities != 1 {
			t.Fatal("failed channel save changed channel or abilities")
		}
	}
}

func TestChannelsConcurrentlyRegisterSharedModelOnce(t *testing.T) {
	newTestDB(t)
	var workers sync.WaitGroup
	results := make(chan error, 8)
	for range 8 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			results <- InsertChannel(&Channel{Models: "shared", Group: "default", Status: 1})
		}()
	}
	workers.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	catalog, err := ReadModelMetadataCatalog(context.Background())
	if err != nil || len(catalog.Models) != 1 || catalog.Revision != 1 || channelRevision(t) != 8 {
		t.Fatalf("concurrent registration: %+v %v", catalog, err)
	}
}

func TestAutomaticModelRegistrationDoesNotPublishUnavailableModels(t *testing.T) {
	newTestDB(t)
	ch := &Channel{Models: "automatic", Group: "default", Status: 1}
	if err := InsertChannel(ch); err != nil {
		t.Fatal(err)
	}
	if names, err := AllPlazaModels(); err != nil || !reflect.DeepEqual(names, []string{"automatic"}) {
		t.Fatalf("live automatic model disappeared: %v %v", names, err)
	}
	if err := DeleteChannel(ch.ID); err != nil {
		t.Fatal(err)
	}
	if names, err := AllPlazaModels(); err != nil || len(names) != 0 {
		t.Fatalf("automatic placeholder published without a route: %v %v", names, err)
	}
	if _, found, err := GetModelMeta("automatic"); err != nil || !found {
		t.Fatal("automatic metadata disappeared from the management catalog")
	}
	if err := UpsertModelMeta(&ModelMeta{Model: "automatic", Description: "Published by an administrator"}); err != nil {
		t.Fatal(err)
	}
	if names, err := AllPlazaModels(); err != nil || !reflect.DeepEqual(names, []string{"automatic"}) {
		t.Fatalf("manual catalog publication changed: %v %v", names, err)
	}
}

func TestChannelModelBackfillIsExplicitAtomicAndIdempotent(t *testing.T) {
	groupConfigurationDB(t)
	ctx := context.Background()
	existing := &ModelMeta{Model: "existing", Description: "Retain this"}
	if err := UpsertModelMeta(existing); err != nil {
		t.Fatal(err)
	}
	// Legacy rows bypass current channel creation to represent an existing database.
	for range 2 {
		if err := DB.Create(&Channel{Models: " existing, legacy, legacy "}).Error; err != nil {
			t.Fatal(err)
		}
	}
	options := groupConfigurationSnapshot(t)
	before, err := ReadModelMetadataCatalog(ctx)
	if err != nil || len(before.Models) != 1 {
		t.Fatalf("legacy data was registered implicitly: %+v %v", before, err)
	}
	if err := DB.Exec(`CREATE TRIGGER reject_backfill BEFORE UPDATE ON model_metadata_states WHEN NEW.revision > OLD.revision BEGIN SELECT RAISE(ABORT, 'synthetic backfill failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if count, err := BackfillChannelModelMetadata(ctx); err == nil || count != 0 {
		t.Fatalf("failed backfill was not atomic: %d %v", count, err)
	}
	after, err := ReadModelMetadataCatalog(ctx)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("failed backfill changed metadata")
	}
	if err := DB.Exec("DROP TRIGGER reject_backfill").Error; err != nil {
		t.Fatal(err)
	}
	for _, want := range []int64{1, 0} {
		if count, err := BackfillChannelModelMetadata(ctx); err != nil || count != want {
			t.Fatalf("backfill: %d %v; want %d", count, err, want)
		}
	}
	stored, _, err := GetModelMeta(existing.Model)
	if err != nil || !reflect.DeepEqual(stored, existing) || channelRevision(t) != 0 || groupConfigurationSnapshot(t) != options {
		t.Fatal("backfill replaced existing metadata or changed channel/billing configuration")
	}
	catalog, err := ReadModelMetadataCatalog(ctx)
	if err != nil || catalog.Revision != before.Revision+1 || len(catalog.Models) != 2 {
		t.Fatalf("backfill catalog: %+v %v", catalog, err)
	}
}

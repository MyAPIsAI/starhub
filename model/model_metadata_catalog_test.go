package model

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"sync"
	"testing"
)

func TestModelMetadataCatalogRevisionSurvivesLegacyWritesAndRecreation(t *testing.T) {
	path := groupConfigurationDB(t)
	ctx := context.Background()
	state, err := ReadModelMetadataCatalog(ctx)
	if err != nil || state.Revision != 0 || state.Models == nil || len(state.Models) != 0 {
		t.Fatalf("empty catalog: %+v %v", state, err)
	}
	options := groupConfigurationSnapshot(t)
	meta := &ModelMeta{Model: "guarded", Description: "First", Tags: "function_call,unknown-extension", MaxOutput: 2048}
	if revision, err := UpsertModelMetaIfRevision(ctx, meta, 0); err != nil || revision != 1 {
		t.Fatalf("create: %d %v", revision, err)
	}
	meta.Description = "Legacy update"
	if err := UpsertModelMeta(meta); err != nil {
		t.Fatal(err)
	}
	for _, write := range []func() error{func() error { _, err := UpsertModelMetaIfRevision(ctx, meta, 1); return err }, func() error { _, err := DeleteModelMetaIfRevision(ctx, meta.Model, 1); return err }} {
		if err := write(); !errors.Is(err, ErrModelMetadataConflict) {
			t.Fatalf("legacy writer did not invalidate old precondition: %v", err)
		}
	}
	if revision, err := DeleteModelMetaIfRevision(ctx, meta.Model, 2); err != nil || revision != 3 {
		t.Fatalf("delete: %d %v", revision, err)
	}
	if revision, err := DeleteModelMetaIfRevision(ctx, meta.Model, 3); err != nil || revision != 3 {
		t.Fatal("missing-row delete should not advance revision")
	}
	meta.CreatedAt = 0
	if revision, err := UpsertModelMetaIfRevision(ctx, meta, 3); err != nil || revision != 4 {
		t.Fatalf("recreate: %d %v", revision, err)
	}
	if _, err := DeleteModelMetaIfRevision(ctx, meta.Model, 2); !errors.Is(err, ErrModelMetadataConflict) {
		t.Fatal("stale deletion removed a recreated row")
	}
	if groupConfigurationSnapshot(t) != options {
		t.Fatal("metadata catalog changed billing options, group metadata or history")
	}
	before, err := ReadModelMetadataCatalog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	after, err := ReadModelMetadataCatalog(ctx)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("metadata revision or rows did not survive restart")
	}
}

func TestModelMetadataCounterFailureRollsBackCreateEditAndDelete(t *testing.T) {
	groupConfigurationDB(t)
	ctx := context.Background()
	meta := &ModelMeta{Model: "existing", Description: "Original"}
	if err := UpsertModelMeta(meta); err != nil {
		t.Fatal(err)
	}
	before, err := ReadModelMetadataCatalog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := DB.Exec(`CREATE TRIGGER reject_metadata_revision BEFORE UPDATE ON model_metadata_states WHEN NEW.revision > OLD.revision BEGIN SELECT RAISE(ABORT, 'synthetic revision failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	for _, write := range []func() error{func() error { return UpsertModelMeta(&ModelMeta{Model: "new", Description: "Must roll back"}) }, func() error {
		_, err := UpsertModelMetaIfRevision(ctx, &ModelMeta{Model: meta.Model, Description: "Must roll back"}, 1)
		return err
	}, func() error { _, err := DeleteModelMetaIfRevision(ctx, meta.Model, 1); return err }} {
		if err := write(); err == nil {
			t.Fatal("counter failure was ignored")
		}
		after, err := ReadModelMetadataCatalog(ctx)
		if err != nil || !reflect.DeepEqual(before, after) {
			t.Fatal("row mutation escaped a failed revision transaction")
		}
	}
}

func TestModelMetadataConcurrentWritersAndCoherentReads(t *testing.T) {
	groupConfigurationDB(t)
	ctx := context.Background()
	results := make(chan error, 8)
	var writers sync.WaitGroup
	for range 8 {
		writers.Add(1)
		go func() {
			defer writers.Done()
			_, err := UpsertModelMetaIfRevision(ctx, &ModelMeta{Model: "paired", Description: "1"}, 0)
			results <- err
		}()
	}
	writers.Wait()
	close(results)
	winners := 0
	for err := range results {
		if err == nil {
			winners++
		} else if !errors.Is(err, ErrModelMetadataConflict) {
			t.Fatal(err)
		}
	}
	if winners != 1 {
		t.Fatalf("concurrent winners: %d", winners)
	}
	writeErrors := make(chan error, 1)
	writers.Add(1)
	go func() {
		defer writers.Done()
		for i := int64(1); i <= 30; i++ {
			if _, err := UpsertModelMetaIfRevision(ctx, &ModelMeta{Model: "paired", Description: fmt.Sprint(i + 1)}, i); err != nil {
				writeErrors <- err
				return
			}
		}
	}()
	for range 100 {
		state, err := ReadModelMetadataCatalog(ctx)
		if err != nil || len(state.Models) != 1 {
			t.Errorf("snapshot: %+v %v", state, err)
			break
		}
		value, err := strconv.ParseInt(state.Models[0].Description, 10, 64)
		if err != nil || value != state.Revision {
			t.Errorf("row and revision disagree: %+v", state)
			break
		}
	}
	writers.Wait()
	select {
	case err := <-writeErrors:
		t.Fatal(err)
	default:
	}
}

func TestModelMetadataMigrationCancellationAndRevisionBounds(t *testing.T) {
	path := groupConfigurationDB(t)
	ctx := context.Background()
	if err := UpsertModelMeta(&ModelMeta{Model: "legacy", Tags: "unknown-extension"}); err != nil {
		t.Fatal(err)
	}
	if err := DB.Migrator().DropTable(&ModelMetadataState{}); err != nil {
		t.Fatal(err)
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	state, err := ReadModelMetadataCatalog(ctx)
	if err != nil || state.Revision != 0 || len(state.Models) != 1 || state.Models[0].Tags != "unknown-extension" {
		t.Fatalf("legacy migration lost metadata: %+v %v", state, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if result, err := ReadModelMetadataCatalog(cancelled); !errors.Is(err, context.Canceled) || result != nil {
		t.Fatal("cancelled read returned authoritative state")
	}
	if _, err := UpsertModelMetaIfRevision(cancelled, &ModelMeta{Model: "cancelled"}, 0); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled write was accepted")
	}
	for _, revision := range []int64{-1, maxModelMetadataRevision, maxModelMetadataRevision + 1} {
		if _, err := UpsertModelMetaIfRevision(ctx, &ModelMeta{Model: "invalid"}, revision); !errors.Is(err, ErrInvalidModelMetadataRevision) {
			t.Fatal("invalid precondition accepted")
		}
	}
	if err := DB.Model(&ModelMetadataState{}).Where("id = 1").Update("revision", maxModelMetadataRevision-1).Error; err != nil {
		t.Fatal(err)
	}
	if revision, err := UpsertModelMetaIfRevision(ctx, &ModelMeta{Model: "last-safe"}, maxModelMetadataRevision-1); err != nil || revision != maxModelMetadataRevision {
		t.Fatalf("last safe revision: %d %v", revision, err)
	}
	if err := UpsertModelMeta(&ModelMeta{Model: "overflow"}); err == nil {
		t.Fatal("legacy write overflowed the safe revision")
	}
	if err := DB.Model(&ModelMetadataState{}).Where("id = 1").Update("revision", maxModelMetadataRevision+1).Error; err != nil {
		t.Fatal(err)
	}
	if result, err := ReadModelMetadataCatalog(ctx); err == nil || result != nil {
		t.Fatal("unsafe stored revision accepted")
	}
}

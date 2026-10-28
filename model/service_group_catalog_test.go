package model

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"

	"starhub/setting"
)

func TestGroupCatalogHasNoInventedDefinitionsAndPreservesConfiguredZeros(t *testing.T) {
	groupConfigurationDB(t)
	ctx := context.Background()
	if _, err := SaveOption(setting.OptionKeyGroupRatio, `{"default":1,"legacy-only":0}`, 1); err != nil {
		t.Fatal(err)
	}
	catalog, err := ReadServiceGroupCatalog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Groups == nil || len(catalog.Groups) != 0 || catalog.OptionVersion != 1 || len(catalog.GroupRatios) != 2 {
		t.Fatalf("catalog invented metadata: %+v", catalog)
	}
	if value, present := catalog.GroupRatios["legacy-only"]; !present || value != 0 {
		t.Fatal("configured zero lost")
	}
	before := groupConfigurationSnapshot(t)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := ReadServiceGroupCatalog(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled read: %v", err)
	}
	if groupConfigurationSnapshot(t) != before {
		t.Fatal("read changed configuration")
	}
	if err := UpsertOption(setting.OptionKeyGroupRatio, `invalid`); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadServiceGroupCatalog(ctx); err == nil {
		t.Fatal("corrupt stored ratios returned as authoritative")
	}
}

func TestGroupStateGuardIncludesIndependentOptionChanges(t *testing.T) {
	groupConfigurationDB(t)
	ctx := context.Background()
	group := &ServiceGroupConfig{GroupID: "guarded", Name: "Original", Multiplier: "0.8", Enabled: true}
	if err := SaveServiceGroupAndRatioIfState(ctx, group, 1, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	catalog, err := ReadServiceGroupCatalog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SaveOption(setting.OptionKeyModelRatio, `{"gpt-4o":12}`, 2); err != nil {
		t.Fatal(err)
	}
	before := groupConfigurationSnapshot(t)
	edit := *group
	edit.Name = "Must not save"
	edit.Multiplier = "0.6"
	if err := SaveServiceGroupAndRatioIfState(ctx, &edit, 1, group.ID, group.Revision, catalog.OptionVersion); !errors.Is(err, ErrOptionVersionConflict) {
		t.Fatalf("independent price edit not protected: %v", err)
	}
	if err := DeleteServiceGroupAndRatioIfState(ctx, group.GroupID, 1, group.ID, group.Revision, catalog.OptionVersion); !errors.Is(err, ErrOptionVersionConflict) {
		t.Fatalf("independent price delete not protected: %v", err)
	}
	if groupConfigurationSnapshot(t) != before {
		t.Fatal("stale option guard changed state")
	}
	current, err := ReadServiceGroupCatalog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveServiceGroupAndRatioIfState(ctx, &edit, 1, group.ID, group.Revision, current.OptionVersion); err != nil {
		t.Fatal(err)
	}
	if edit.Revision != group.Revision+1 || storedGroupRatios(t)[group.GroupID] != 0.6 {
		t.Fatal("current combined guard did not save")
	}
	before = groupConfigurationSnapshot(t)
	for _, version := range []int64{-1, maxServiceGroupRevision, maxServiceGroupRevision + 1} {
		if err := SaveServiceGroupAndRatioIfState(ctx, &edit, 1, edit.ID, edit.Revision, version); !errors.Is(err, ErrInvalidServiceGroupPrecondition) {
			t.Fatalf("invalid option version: %v", err)
		}
	}
	if groupConfigurationSnapshot(t) != before {
		t.Fatal("invalid version changed state")
	}
}

func TestGroupCatalogReadsOneSnapshotDuringConcurrentUpdates(t *testing.T) {
	groupConfigurationDB(t)
	ctx := context.Background()
	group := &ServiceGroupConfig{GroupID: "managed", Name: "Managed", Multiplier: "0.8", Enabled: true}
	if err := SaveServiceGroupAndRatio(ctx, group, 1); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	failures := make(chan error, 1)
	wg.Add(1)
	defer wg.Wait()
	go func() {
		defer wg.Done()
		for i := 0; i < 40; i++ {
			rate := "0.5"
			if i%2 == 0 {
				rate = "0.6"
			}
			if err := SaveServiceGroupAndRatio(ctx, &ServiceGroupConfig{GroupID: group.GroupID, Name: "Managed", Multiplier: rate, Enabled: true}, 1); err != nil {
				failures <- err
				return
			}
		}
	}()
	for i := 0; i < 80; i++ {
		catalog, err := ReadServiceGroupCatalog(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(catalog.Groups) != 1 {
			t.Fatal("snapshot lost metadata")
		}
		rate, err := strconv.ParseFloat(catalog.Groups[0].Multiplier, 64)
		if err != nil {
			t.Fatal(err)
		}
		if catalog.GroupRatios[group.GroupID] != rate {
			t.Fatal("metadata and ratios came from different snapshots")
		}
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
}

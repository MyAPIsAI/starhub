package model

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
)

func TestGroupRevisionConcurrentCreateAndUpdateHaveOneWinner(t *testing.T) {
	for _, creating := range []bool{true, false} {
		t.Run(fmt.Sprint(creating), func(t *testing.T) {
			groupConfigurationDB(t)
			ctx := context.Background()
			expectedID, expectedRevision := int64(0), int64(0)
			if !creating {
				group := &ServiceGroupConfig{GroupID: "guarded", Name: "Original", Multiplier: "0.8", Enabled: true}
				if err := SaveServiceGroupAndRatio(ctx, group, 1); err != nil {
					t.Fatal(err)
				}
				expectedID, expectedRevision = group.ID, group.Revision
			}
			var wg sync.WaitGroup
			start := make(chan struct{})
			results := make(chan error, 8)
			for i := 0; i < 8; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					<-start
					results <- SaveServiceGroupAndRatioIfRevision(ctx, &ServiceGroupConfig{GroupID: "guarded", Name: fmt.Sprintf("Writer %d", i), Multiplier: "0.6", Enabled: true}, int64(i+1), expectedID, expectedRevision)
				}(i)
			}
			close(start)
			wg.Wait()
			close(results)
			wins := 0
			for err := range results {
				if err == nil {
					wins++
				} else if !errors.Is(err, ErrServiceGroupConflict) {
					t.Fatal(err)
				}
			}
			if wins != 1 {
				t.Fatalf("successful stale writes: %d", wins)
			}
			group, err := GetServiceGroup("guarded")
			if err != nil || group.Revision != expectedRevision+1 {
				t.Fatalf("stored revision: %+v %v", group, err)
			}
		})
	}
}

func TestGroupRevisionProtectsDeletesAndRecreatedIdentity(t *testing.T) {
	groupConfigurationDB(t)
	ctx := context.Background()
	group := &ServiceGroupConfig{GroupID: "guarded", Name: "Original", Multiplier: "0.8", Enabled: true}
	if err := SaveServiceGroupAndRatioIfRevision(ctx, group, 1, 0, 0); err != nil {
		t.Fatal(err)
	}
	id, revision := group.ID, group.Revision
	group.Name = "New name"
	if err := SaveServiceGroupAndRatio(ctx, group, 2); err != nil {
		t.Fatal(err)
	}
	before := groupConfigurationSnapshot(t)
	if err := DeleteServiceGroupAndRatioIfRevision(ctx, group.GroupID, 1, id, revision); !errors.Is(err, ErrServiceGroupConflict) {
		t.Fatalf("stale delete: %v", err)
	}
	if groupConfigurationSnapshot(t) != before {
		t.Fatal("stale delete changed state")
	}
	if err := DeleteServiceGroupAndRatioIfRevision(ctx, group.GroupID, 1, group.ID, group.Revision); err != nil {
		t.Fatal(err)
	}
	recreated := &ServiceGroupConfig{GroupID: group.GroupID, Name: "Recreated", Multiplier: "0.7", Enabled: true}
	if err := SaveServiceGroupAndRatioIfRevision(ctx, recreated, 1, 0, 0); err != nil {
		t.Fatal(err)
	}
	if recreated.ID == id || recreated.Revision != revision {
		t.Fatal("fixture did not recreate a new identity at the same revision")
	}
	before = groupConfigurationSnapshot(t)
	stale := &ServiceGroupConfig{GroupID: group.GroupID, Name: "Stale", Multiplier: "0.2", Enabled: true}
	if err := SaveServiceGroupAndRatioIfRevision(ctx, stale, 1, id, revision); !errors.Is(err, ErrServiceGroupConflict) {
		t.Fatalf("stale recreated save: %v", err)
	}
	if err := DeleteServiceGroupAndRatioIfRevision(ctx, group.GroupID, 1, id, revision); !errors.Is(err, ErrServiceGroupConflict) {
		t.Fatalf("stale recreated delete: %v", err)
	}
	if groupConfigurationSnapshot(t) != before {
		t.Fatal("old identity changed recreated configuration")
	}
}

func TestGroupRevisionMigrationAndBounds(t *testing.T) {
	path := groupConfigurationDB(t)
	group := &ServiceGroupConfig{GroupID: "legacy", Name: "Legacy", Multiplier: "0.8", Enabled: true}
	if err := SaveServiceGroupAndRatio(context.Background(), group, 1); err != nil {
		t.Fatal(err)
	}
	if err := DB.Migrator().DropColumn(&ServiceGroupConfig{}, "Revision"); err != nil {
		t.Fatal(err)
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	old, err := GetServiceGroup(group.GroupID)
	if err != nil || old.Revision != 0 || old.ID != group.ID || old.Name != "Legacy" {
		t.Fatalf("migration: %+v %v", old, err)
	}
	if err := SaveServiceGroupAndRatioIfRevision(context.Background(), old, 1, old.ID, 0); err != nil || old.Revision != 1 {
		t.Fatalf("initial migrated edit: %+v %v", old, err)
	}
	before := groupConfigurationSnapshot(t)
	for _, pair := range [][2]int64{{-1, 0}, {0, 1}, {1, -1}, {1, maxServiceGroupRevision}, {maxServiceGroupRevision + 1, 0}} {
		if err := SaveServiceGroupAndRatioIfRevision(context.Background(), old, 1, pair[0], pair[1]); !errors.Is(err, ErrInvalidServiceGroupPrecondition) {
			t.Fatalf("invalid precondition %+v: %v", pair, err)
		}
	}
	if groupConfigurationSnapshot(t) != before {
		t.Fatal("invalid preconditions changed state")
	}
}

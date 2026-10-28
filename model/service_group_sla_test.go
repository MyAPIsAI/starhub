package model

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

func slaValue(value int64) *int64 { return &value }

func TestServiceLevelTargetsPersistAndRemainSeparateFromBilling(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sla.db")
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	group := &ServiceGroupConfig{GroupID: "target-group", Name: "Target group", Multiplier: "0.8", Enabled: true}
	if err := UpsertServiceGroup(group); err != nil {
		t.Fatal(err)
	}
	if group.ID < 1 || group.ServiceLevel().Targets != nil || group.SLARevision != 0 {
		t.Fatalf("fabricated initial target: %+v", group)
	}
	before, err := OptionVersion()
	if err != nil {
		t.Fatal(err)
	}
	updated, err := UpdateServiceGroupSLA(context.Background(), group.GroupID, group.ID, 0, &ServiceGroupSLATarget{SuccessRatePPM: slaValue(995000), P95LatencyMS: slaValue(800)})
	if err != nil {
		t.Fatal(err)
	}
	if updated.SLARevision != 1 || updated.SLAUpdatedAt < 1 || *updated.SLASuccessPPM != 995000 || *updated.SLAP95MS != 800 || updated.Multiplier != "0.8" {
		t.Fatalf("unexpected target: %+v", updated)
	}
	if version, _ := OptionVersion(); version != before {
		t.Fatal("publishing metadata changed pricing revision")
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	reopened, err := GetServiceGroup(group.GroupID)
	if err != nil || !reflect.DeepEqual(reopened, updated) {
		t.Fatalf("target lost on reopen: %+v %v", reopened, err)
	}
	metadata := &ServiceGroupConfig{GroupID: group.GroupID, Name: "Renamed group", Multiplier: "0.9", Enabled: true}
	if err := UpsertServiceGroup(metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.ID != group.ID || metadata.SLARevision != 1 || metadata.SLASuccessPPM == nil || *metadata.SLASuccessPPM != 995000 {
		t.Fatal("ordinary group edit cleared target or returned stale metadata")
	}
	zero, err := UpdateServiceGroupSLA(context.Background(), group.GroupID, group.ID, 1, &ServiceGroupSLATarget{SuccessRatePPM: slaValue(0)})
	if err != nil || zero.ServiceLevel().Targets == nil || zero.SLAP95MS != nil || *zero.SLASuccessPPM != 0 {
		t.Fatalf("zero and unknown conflated: %+v %v", zero, err)
	}
	cleared, err := UpdateServiceGroupSLA(context.Background(), group.GroupID, group.ID, 2, nil)
	if err != nil || cleared.ServiceLevel().Targets != nil || cleared.SLARevision != 3 {
		t.Fatalf("unpublish failed: %+v %v", cleared, err)
	}
}

func TestServiceLevelTargetCASValidationAndRecreatedIdentity(t *testing.T) {
	newTestDB(t)
	group := &ServiceGroupConfig{GroupID: "target-group", Name: "Target group", Multiplier: "1", Enabled: true}
	if err := UpsertServiceGroup(group); err != nil {
		t.Fatal(err)
	}
	for _, target := range []*ServiceGroupSLATarget{{}, {SuccessRatePPM: slaValue(-1)}, {SuccessRatePPM: slaValue(1000001)}, {P95LatencyMS: slaValue(0)}, {P95LatencyMS: slaValue(86400001)}} {
		if _, err := UpdateServiceGroupSLA(context.Background(), group.GroupID, group.ID, 0, target); !errors.Is(err, ErrInvalidSLATarget) {
			t.Fatalf("invalid target accepted: %v", err)
		}
	}
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := UpdateServiceGroupSLA(context.Background(), group.GroupID, group.ID, 0, &ServiceGroupSLATarget{P95LatencyMS: slaValue(int64(i + 1))})
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, ErrSLATargetConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 7 {
		t.Fatalf("concurrent outcomes: %d %d", success, conflict)
	}
	if err := DeleteServiceGroup(group.GroupID); err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateServiceGroupSLA(context.Background(), group.GroupID, group.ID, 1, nil); !errors.Is(err, ErrServiceGroupNotFound) {
		t.Fatalf("deleted group: %v", err)
	}
	recreated := &ServiceGroupConfig{GroupID: group.GroupID, Name: "New identity", Multiplier: "1"}
	if err := UpsertServiceGroup(recreated); err != nil {
		t.Fatal(err)
	}
	if recreated.ID == group.ID {
		t.Fatal("fixture did not create a new row identity")
	}
	if _, err := UpdateServiceGroupSLA(context.Background(), group.GroupID, group.ID, 0, &ServiceGroupSLATarget{P95LatencyMS: slaValue(1)}); !errors.Is(err, ErrSLATargetConflict) {
		t.Fatalf("old editor overwrote recreated group: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := UpdateServiceGroupSLA(ctx, recreated.GroupID, recreated.ID, 0, nil); err == nil {
		t.Fatal("cancelled target write succeeded")
	}
	after, _ := GetServiceGroup(group.GroupID)
	if after.SLARevision != 0 || after.ServiceLevel().Targets != nil {
		t.Fatal("failed target writes changed new group")
	}
}

func TestServiceLevelTargetMigrationDoesNotInventDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	group := &ServiceGroupConfig{GroupID: "legacy-group", Name: "Legacy", Multiplier: "1"}
	if err := UpsertServiceGroup(group); err != nil {
		t.Fatal(err)
	}
	for _, column := range []string{"SLASuccessPPM", "SLAP95MS", "SLARevision", "SLAUpdatedAt"} {
		if err := DB.Migrator().DropColumn(&ServiceGroupConfig{}, column); err != nil {
			t.Fatal(err)
		}
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	reopened, err := GetServiceGroup(group.GroupID)
	if err != nil || reopened.Name != "Legacy" || reopened.Multiplier != "1" || reopened.SLARevision != 0 || reopened.ServiceLevel().Targets != nil {
		t.Fatalf("legacy data changed: %+v %v", reopened, err)
	}
}

func TestPublishedServiceLevelRejectsInvalidDataWithoutInventingTargets(t *testing.T) {
	newTestDB(t)
	ctx := context.Background()
	if value, err := PublishedServiceGroupSLA(ctx, "missing"); err != nil || value != nil {
		t.Fatalf("missing group: %+v %v", value, err)
	}
	group := &ServiceGroupConfig{GroupID: "public-group", Name: "Public", Multiplier: "1", Enabled: true}
	if err := UpsertServiceGroup(group); err != nil {
		t.Fatal(err)
	}
	if value, err := PublishedServiceGroupSLA(ctx, group.GroupID); err != nil || value == nil || value.Targets != nil {
		t.Fatalf("unpublished group: %+v %v", value, err)
	}
	if err := DB.Model(&ServiceGroupConfig{}).Where("id = ?", group.ID).Updates(map[string]any{"sla_p95_ms": 0, "sla_revision": 1, "sla_updated_at": time.Now().Unix()}).Error; err != nil {
		t.Fatal(err)
	}
	if value, err := PublishedServiceGroupSLA(ctx, group.GroupID); !errors.Is(err, ErrInvalidSLATarget) || value != nil {
		t.Fatalf("invalid target advertised: %+v %v", value, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := PublishedServiceGroupSLA(cancelled, group.GroupID); err == nil {
		t.Fatal("cancelled metadata read succeeded")
	}
}

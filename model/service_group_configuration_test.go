package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"starhub/setting"
)

func groupConfigurationDB(t *testing.T) string {
	t.Helper()
	setting.ResetAllToDefaults()
	path := filepath.Join(t.TempDir(), "group-configuration.db")
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = CloseDB(); setting.ResetAllToDefaults() })
	if err := SeedOptions(); err != nil {
		t.Fatal(err)
	}
	return path
}

func groupConfigurationSnapshot(t *testing.T) string {
	t.Helper()
	groups, err := ListServiceGroups()
	if err != nil {
		t.Fatal(err)
	}
	options, err := AllOptions()
	if err != nil {
		t.Fatal(err)
	}
	changes, err := ListOptionChanges(100)
	if err != nil {
		t.Fatal(err)
	}
	version, err := OptionVersion()
	if err != nil {
		t.Fatal(err)
	}
	var deletions []ServiceGroupDeletion
	if err := DB.Order("group_id").Find(&deletions).Error; err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal([]any{groups, options, changes, version, deletions})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func storedGroupRatios(t *testing.T) map[string]float64 {
	t.Helper()
	value, present, err := GetOption(setting.OptionKeyGroupRatio)
	if err != nil || !present {
		t.Fatalf("group ratio unavailable: %v", err)
	}
	var result map[string]float64
	if err := json.Unmarshal([]byte(value), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestGroupConfigurationPreservesUnrelatedPricesTargetsAndHistory(t *testing.T) {
	path := groupConfigurationDB(t)
	if err := UpsertServiceGroup(&ServiceGroupConfig{GroupID: "independent", Name: "Metadata", Multiplier: "1.1", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveOption(setting.OptionKeyGroupRatio, `{"default":0.9,"legacy-only":0,"independent":0.2}`, 9); err != nil {
		t.Fatal(err)
	}
	group := &ServiceGroupConfig{GroupID: "managed", Name: "Managed", Multiplier: "0.8", Enabled: true}
	ctx := context.Background()
	if err := SaveServiceGroupAndRatio(ctx, group, 41); err != nil {
		t.Fatal(err)
	}
	ratios := storedGroupRatios(t)
	if len(ratios) != 4 || ratios["managed"] != 0.8 || ratios["default"] != 0.9 || ratios["independent"] != 0.2 {
		t.Fatalf("unrelated prices overwritten: %+v", ratios)
	}
	if value, ok := ratios["legacy-only"]; !ok || value != 0 {
		t.Fatal("explicit legacy zero lost")
	}
	updated, err := UpdateServiceGroupSLA(ctx, group.GroupID, group.ID, 0, &ServiceGroupSLATarget{SuccessRatePPM: slaValue(995000)})
	if err != nil {
		t.Fatal(err)
	}
	group.Name = "Renamed"
	group.Multiplier = "0.6"
	if err := SaveServiceGroupAndRatio(ctx, group, 42); err != nil {
		t.Fatal(err)
	}
	if group.ID != updated.ID || group.SLARevision != 1 || group.SLASuccessPPM == nil || *group.SLASuccessPPM != 995000 {
		t.Fatal("ordinary atomic save cleared SLA")
	}
	changes, err := ListOptionChanges(100)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 3 || changes[0].ActorID != 42 || changes[1].ActorID != 41 {
		t.Fatalf("wrong option attribution: %+v", changes)
	}
	group.Enabled = false
	if err := SaveServiceGroupAndRatio(ctx, group, 42); err != nil {
		t.Fatal(err)
	}
	if _, ok := storedGroupRatios(t)["managed"]; ok {
		t.Fatal("disabled group retained a billing ratio")
	}
	group.Enabled = true
	if err := SaveServiceGroupAndRatio(ctx, group, 42); err != nil {
		t.Fatal(err)
	}
	if err := DeleteServiceGroupAndRatio(ctx, group.GroupID, 43); err != nil {
		t.Fatal(err)
	}
	if _, err := GetServiceGroup(group.GroupID); !errors.Is(err, ErrServiceGroupNotFound) {
		t.Fatalf("metadata remained: %v", err)
	}
	ratios = storedGroupRatios(t)
	if _, ok := ratios["managed"]; ok || len(ratios) != 3 || ratios["independent"] != 0.2 {
		t.Fatal("delete changed unrelated ratio or retained target")
	}
	before := groupConfigurationSnapshot(t)
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	if groupConfigurationSnapshot(t) != before {
		t.Fatal("configuration changed after reopening")
	}
}

func TestGroupConfigurationRollbackIncludesMetadataOptionsVersionAndHistory(t *testing.T) {
	for _, operation := range []string{"create", "update", "delete"} {
		t.Run(operation, func(t *testing.T) {
			groupConfigurationDB(t)
			group := &ServiceGroupConfig{GroupID: "managed", Name: "Original", Multiplier: "0.8", Enabled: true}
			if operation != "create" {
				if err := SaveServiceGroupAndRatio(context.Background(), group, 1); err != nil {
					t.Fatal(err)
				}
			}
			if err := DB.Exec(`CREATE TRIGGER reject_group_option BEFORE INSERT ON option_changes WHEN NEW.key = 'group_ratio' BEGIN SELECT RAISE(ABORT, 'synthetic history failure'); END`).Error; err != nil {
				t.Fatal(err)
			}
			before := groupConfigurationSnapshot(t)
			var err error
			if operation == "delete" {
				err = DeleteServiceGroupAndRatio(context.Background(), group.GroupID, 2)
			} else {
				group.Name = "Must roll back"
				group.Multiplier = "0.6"
				err = SaveServiceGroupAndRatio(context.Background(), group, 2)
			}
			if err == nil {
				t.Fatal("injected storage failure acknowledged")
			}
			if groupConfigurationSnapshot(t) != before {
				t.Fatal("transaction leaked metadata, option, version or history")
			}
		})
	}
}

func TestGroupConfigurationConcurrentEditsDoNotLoseUnrelatedRatios(t *testing.T) {
	groupConfigurationDB(t)
	if _, err := SaveOption(setting.OptionKeyGroupRatio, `{"default":1,"legacy-only":0.3}`, 1); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			failures <- SaveServiceGroupAndRatio(context.Background(), &ServiceGroupConfig{GroupID: fmt.Sprintf("writer-%d", i), Name: "Writer", Multiplier: "0.5", Enabled: true}, int64(i+10))
		}(i)
	}
	close(start)
	wg.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	ratios := storedGroupRatios(t)
	if len(ratios) != 10 || ratios["legacy-only"] != 0.3 {
		t.Fatalf("lost concurrent price: %+v", ratios)
	}
	for i := 0; i < 8; i++ {
		if ratios[fmt.Sprintf("writer-%d", i)] != 0.5 {
			t.Fatal("writer ratio lost")
		}
	}
	version, err := OptionVersion()
	if err != nil || version != 9 {
		t.Fatalf("version=%d error=%v", version, err)
	}
}

func TestGroupConfigurationCancellationInvalidStoreAndExplicitDefaultDisable(t *testing.T) {
	groupConfigurationDB(t)
	group := &ServiceGroupConfig{GroupID: "managed", Name: "Managed", Multiplier: "0.8", Enabled: true}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	before := groupConfigurationSnapshot(t)
	if err := SaveServiceGroupAndRatio(ctx, group, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled save: %v", err)
	}
	if groupConfigurationSnapshot(t) != before {
		t.Fatal("cancelled save mutated state")
	}
	if err := UpsertOption(setting.OptionKeyGroupRatio, `{"broken"`); err != nil {
		t.Fatal(err)
	}
	before = groupConfigurationSnapshot(t)
	if err := SaveServiceGroupAndRatio(context.Background(), group, 1); err == nil {
		t.Fatal("invalid stored option accepted")
	}
	if groupConfigurationSnapshot(t) != before {
		t.Fatal("invalid stored option leaked metadata")
	}
	if _, err := SaveOption(setting.OptionKeyGroupRatio, `{"default":1,"legacy-only":0.3}`, 1); err != nil {
		t.Fatal(err)
	}
	def := &ServiceGroupConfig{GroupID: "default", Name: "Default", Multiplier: "1", Enabled: false}
	if err := SaveServiceGroupAndRatio(context.Background(), def, 1); err != nil {
		t.Fatal(err)
	}
	if _, ok := storedGroupRatios(t)["default"]; ok {
		t.Fatal("explicit default disable silently reenabled")
	}
	if err := SaveServiceGroupAndRatio(context.Background(), group, 1); err != nil {
		t.Fatal(err)
	}
	if _, ok := storedGroupRatios(t)["default"]; ok {
		t.Fatal("unrelated group edit invented default access")
	}
	def.Enabled = true
	if err := SaveServiceGroupAndRatio(context.Background(), def, 1); err != nil {
		t.Fatal(err)
	}
	if storedGroupRatios(t)["default"] != 1 {
		t.Fatal("explicit default enable failed")
	}
}

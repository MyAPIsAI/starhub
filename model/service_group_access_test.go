package model

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	"starhub/setting"
)

func TestServiceGroupAccessUsesExactServerIdentityAndFailsClosed(t *testing.T) {
	groupConfigurationDB(t)
	for _, group := range []ServiceGroupConfig{
		{GroupID: "common", Enabled: true, Visibility: "common", AccountIDs: "999"},
		{GroupID: "disabled", Enabled: false, Visibility: "common"},
		{GroupID: "private", Enabled: true, Visibility: "custom", AccountIDs: " 42 ,043,+44,4e1,acct-45,0,-1,9223372036854775807"},
		{GroupID: "unknown", Enabled: true, Visibility: "future"},
		{GroupID: "empty", Enabled: true, Visibility: "custom"},
	} {
		if err := UpsertServiceGroup(&group); err != nil {
			t.Fatal(err)
		}
	}
	before := groupConfigurationSnapshot(t)
	groups := []string{"", "legacy", "common", "disabled", "private", "unknown", "empty", "private"}
	for _, identity := range []int64{0, -1, 42, 43, 44, 40, 45, 9223372036854775807} {
		access, err := ServiceGroupAccess(context.Background(), groups, identity)
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]bool{"legacy": true, "common": true, "disabled": false, "private": identity == 42 || identity == 9223372036854775807, "unknown": false, "empty": false}
		if !reflect.DeepEqual(access, want) {
			t.Fatalf("identity %d: %v, want %v", identity, access, want)
		}
	}
	if groupConfigurationSnapshot(t) != before {
		t.Fatal("authorization read changed configuration")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if access, err := ServiceGroupAccess(cancelled, groups, 42); !errors.Is(err, context.Canceled) || access != nil {
		t.Fatalf("cancelled read granted access: %v %v", access, err)
	}
	if err := DB.Migrator().DropTable(&ServiceGroupConfig{}); err != nil {
		t.Fatal(err)
	}
	if access, err := ServiceGroupAccess(context.Background(), groups, 42); err == nil || access != nil {
		t.Fatalf("failed read granted legacy access: %v %v", access, err)
	}
}

func TestDeletedGroupCannotReopenThroughStaleRatiosAndRecreationIsAtomic(t *testing.T) {
	path := groupConfigurationDB(t)
	ctx := context.Background()
	group := &ServiceGroupConfig{GroupID: "private", Name: "Private", Enabled: true, Visibility: "custom", AccountIDs: "42", Multiplier: "0.5"}
	if err := SaveServiceGroupAndRatio(ctx, group, 1); err != nil {
		t.Fatal(err)
	}
	if err := LoadOptionsToSettings(); err != nil {
		t.Fatal(err)
	}
	assertAccess := func(user int64, want bool) {
		t.Helper()
		access, err := ServiceGroupAccess(ctx, []string{group.GroupID}, user)
		if err != nil || access[group.GroupID] != want {
			t.Fatalf("identity %d access=%v want=%v err=%v", user, access, want, err)
		}
	}
	rejectHistory := func() {
		t.Helper()
		if err := DB.Exec(`CREATE TRIGGER reject_access_change BEFORE INSERT ON option_changes BEGIN SELECT RAISE(ABORT, 'synthetic rollback'); END`).Error; err != nil {
			t.Fatal(err)
		}
	}
	dropTrigger := func() {
		t.Helper()
		if err := DB.Exec("DROP TRIGGER reject_access_change").Error; err != nil {
			t.Fatal(err)
		}
	}
	rejectHistory()
	if err := DeleteServiceGroupAndRatio(ctx, group.GroupID, 1); err == nil {
		t.Fatal("delete ignored option-history failure")
	}
	assertAccess(42, true)
	dropTrigger()
	if err := DeleteServiceGroupAndRatio(ctx, group.GroupID, 1); err != nil {
		t.Fatal(err)
	}
	if !setting.ContainsGroupRatio(group.GroupID) {
		t.Fatal("fixture must retain the process-local ratio from before deletion")
	}
	assertAccess(42, false)
	assertAccess(43, false)
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	assertAccess(42, false)
	rejectHistory()
	if err := SaveServiceGroupAndRatio(ctx, group, 1); err == nil {
		t.Fatal("recreation ignored option-history failure")
	}
	assertAccess(42, false)
	dropTrigger()
	if err := SaveServiceGroupAndRatio(ctx, group, 1); err != nil {
		t.Fatal(err)
	}
	assertAccess(42, true)
	assertAccess(43, false)
}

func TestGroupAccessSnapshotNeverGrantsLegacyAccessDuringRecreation(t *testing.T) {
	groupConfigurationDB(t)
	group := &ServiceGroupConfig{GroupID: "private", Enabled: true, Visibility: "custom", AccountIDs: "42"}
	if err := UpsertServiceGroup(group); err != nil {
		t.Fatal(err)
	}
	var writers sync.WaitGroup
	errors := make(chan error, 1)
	writers.Add(1)
	go func() {
		defer writers.Done()
		for range 30 {
			if err := DeleteServiceGroup(group.GroupID); err != nil {
				errors <- err
				return
			}
			copy := *group
			if err := UpsertServiceGroup(&copy); err != nil {
				errors <- err
				return
			}
		}
	}()
	for range 100 {
		access, err := ServiceGroupAccess(context.Background(), []string{group.GroupID}, 43)
		if err != nil || access[group.GroupID] {
			t.Errorf("recreation transiently granted legacy access: %v %v", access, err)
			break
		}
	}
	writers.Wait()
	select {
	case err := <-errors:
		t.Fatal(err)
	default:
	}
}

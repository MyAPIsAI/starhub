package model

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"
)

func TestDefaultGroupsAreSeededOnceAndPreserveOperatorChanges(t *testing.T) {
	newTestDB(t)
	if err := Seed("bootstrap-test-password"); err != nil {
		t.Fatal(err)
	}
	for id, multiplier := range map[string]string{"ultra": "1", "pro": "0.8", "standard": "0.6", "lite": "0.4"} {
		group, err := GetServiceGroup(id)
		if err != nil || !group.Enabled || group.Visibility != "common" || group.Multiplier != multiplier || group.ServiceLevel().Targets == nil {
			t.Fatalf("invalid default %s: %+v %v", id, group, err)
		}
	}
	ultra, _ := GetServiceGroup("ultra")
	ultra.Enabled, ultra.Multiplier = false, "0.9"
	if err := SaveServiceGroupAndRatio(context.Background(), ultra, 1); err != nil {
		t.Fatal(err)
	}
	if err := DeleteServiceGroupAndRatio(context.Background(), "lite", 1); err != nil {
		t.Fatal(err)
	}
	if err := Seed("unused-restart-password"); err != nil {
		t.Fatal(err)
	}
	fresh, _ := GetServiceGroup("ultra")
	if fresh.Enabled || fresh.Multiplier != "0.9" || fresh.Revision != ultra.Revision {
		t.Fatal("startup overwrote an edited default group")
	}
	if _, err := GetServiceGroup("lite"); !errors.Is(err, ErrServiceGroupNotFound) {
		t.Fatal("startup restored a deleted default group")
	}
}

func TestDefaultGroupsDoNotExpandExistingConfiguration(t *testing.T) {
	newTestDB(t)
	const existing = `{"default":1,"custom":0.25}`
	if err := UpsertOption("group_ratio", existing); err != nil {
		t.Fatal(err)
	}
	group := &ServiceGroupConfig{GroupID: "custom", Name: "Existing", Enabled: false, Visibility: "custom", AccountIDs: "1", Multiplier: "0.25"}
	if err := UpsertServiceGroup(group); err != nil {
		t.Fatal(err)
	}
	if err := Seed("bootstrap-test-password"); err != nil {
		t.Fatal(err)
	}
	groups, _ := ListServiceGroups()
	value, _, _ := GetOption("group_ratio")
	if len(groups) != 1 || value != existing || groups[0].Enabled || groups[0].Visibility != "custom" || groups[0].Revision != group.Revision {
		t.Fatal("bootstrap expanded or replaced existing group policy")
	}
}

func TestDefaultGroupBootstrapRollsBackWithRootCreation(t *testing.T) {
	newTestDB(t)
	const callback = "test:fail_default_group"
	if err := DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if group, ok := tx.Statement.Dest.(*ServiceGroupConfig); ok && group.GroupID == "pro" {
			tx.AddError(errors.New("injected bootstrap failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer DB.Callback().Create().Remove(callback)
	if err := Seed("bootstrap-test-password"); err == nil {
		t.Fatal("bootstrap failure was ignored")
	}
	var users, groups int64
	DB.Model(&User{}).Count(&users)
	DB.Model(&ServiceGroupConfig{}).Count(&groups)
	if users != 0 || groups != 0 {
		t.Fatal("failed bootstrap committed partial state")
	}
	DB.Callback().Create().Remove(callback)
	if err := Seed("bootstrap-test-password"); err != nil {
		t.Fatal(err)
	}
}

package model

import (
	"context"
	"errors"
	"testing"

	"starhub/common"
)

func assertManagedGroupIdentity(t *testing.T, ch *Channel, allowed bool) {
	t.Helper()
	got, err := GetSatisfiedChannel("default", "alpha")
	if allowed {
		if err != nil || got.ID != ch.ID {
			t.Fatalf("expected live identity: %+v %v", got, err)
		}
	} else if !errors.Is(err, ErrNoChannel) {
		t.Fatalf("stale group identity routed: %+v %v", got, err)
	}
	policy, managed, err := ReadResourceGatewayPolicy(context.Background(), ch.ID, "default", "alpha")
	if err != nil || !managed || policy.Allows("/v1/chat/completions", nil, 1, 1) != allowed {
		t.Fatalf("policy disagrees with group identity: %v", err)
	}
	models, err := ListModelsByGroup("default")
	if err != nil || (len(models) > 0) != allowed {
		t.Fatalf("discovery disagrees: %v %v", models, err)
	}
	coverage, err := ListModelCoverage()
	if err != nil || (len(coverage) > 0) != allowed {
		t.Fatalf("coverage disagrees: %+v %v", coverage, err)
	}
	names, err := AllPlazaModels()
	if err != nil || (len(names) > 0) != allowed {
		t.Fatalf("plaza disagrees: %v %v", names, err)
	}
}

func TestResourceGatewayGroupIdentityDoesNotFollowRecreatedName(t *testing.T) {
	newTestDB(t)
	ch := addChannel(t, "managed", "default", "alpha", 0, 1, common.ChannelStatusEnabled)
	managedGatewayFixture(t, ch, true, ResourceGatewayRoute{Model: "alpha", Group: "default"})
	assertManagedGroupIdentity(t, ch, true)
	group := &ServiceGroupConfig{GroupID: "default", Name: "First definition", Enabled: true, Visibility: "common"}
	if err := UpsertServiceGroup(group); err != nil {
		t.Fatal(err)
	}
	// A former legacy grant cannot silently attach to a newly defined group.
	assertManagedGroupIdentity(t, ch, false)
	if err := DB.Model(&ResourceGatewayRoute{}).Where("channel_id = ?", ch.ID).Update("group_definition_id", group.ID).Error; err != nil {
		t.Fatal(err)
	}
	assertManagedGroupIdentity(t, ch, true)
	group.Name = "Renamed same identity"
	if err := UpsertServiceGroup(group); err != nil {
		t.Fatal(err)
	}
	assertManagedGroupIdentity(t, ch, true)
	firstID := group.ID
	if err := DeleteServiceGroup("default"); err != nil {
		t.Fatal(err)
	}
	assertManagedGroupIdentity(t, ch, false)
	if err := SetAbilitiesEnabledByChannel(ch.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := UpdateChannelStatus(ch.ID, common.ChannelStatusEnabled); err != nil {
		t.Fatal(err)
	}
	assertManagedGroupIdentity(t, ch, false)
	recreated := &ServiceGroupConfig{GroupID: "default", Name: "First definition", Enabled: true, Visibility: "common"}
	if err := UpsertServiceGroup(recreated); err != nil {
		t.Fatal(err)
	}
	if recreated.ID == firstID {
		t.Fatal("test did not recreate a distinct identity")
	}
	assertManagedGroupIdentity(t, ch, false)
	if err := RebuildAbilities(ch); err != nil {
		t.Fatal(err)
	}
	assertManagedGroupIdentity(t, ch, false)
}

func TestResourceGatewayCurrentGroupRestrictionsRemainLive(t *testing.T) {
	newTestDB(t)
	ch := addChannel(t, "managed", "default", "alpha", 0, 1, common.ChannelStatusEnabled)
	group := &ServiceGroupConfig{GroupID: "default", Name: "Defined", Enabled: true, Visibility: "common"}
	if err := UpsertServiceGroup(group); err != nil {
		t.Fatal(err)
	}
	managedGatewayFixture(t, ch, true, ResourceGatewayRoute{Model: "alpha", Group: "default", GroupDefinitionID: group.ID})
	assertManagedGroupIdentity(t, ch, true)
	group.Enabled = false
	if err := UpsertServiceGroup(group); err != nil {
		t.Fatal(err)
	}
	assertManagedGroupIdentity(t, ch, false)
	group.Enabled = true
	if err := UpsertServiceGroup(group); err != nil {
		t.Fatal(err)
	}
	assertManagedGroupIdentity(t, ch, true)
	if err := DB.Model(&ServiceGroupConfig{}).Where("id = ?", group.ID).Update("visibility", "unknown").Error; err != nil {
		t.Fatal(err)
	}
	assertManagedGroupIdentity(t, ch, false)
}

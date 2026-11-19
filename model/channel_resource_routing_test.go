package model

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"

	"starhub/common"
)

func TestChannelResourceGateFiltersBeforeCandidateLimit(t *testing.T) {
	newTestDB(t)
	var off []ChannelResourceDisable
	for i := 0; i < 64; i++ {
		ch := addChannel(t, fmt.Sprintf("closed-%d", i), "default", "alpha", 10, 1, common.ChannelStatusEnabled)
		off = append(off, ChannelResourceDisable{ChannelID: ch.ID, Model: "alpha"})
	}
	if err := DB.Create(&off).Error; err != nil {
		t.Fatal(err)
	}
	live := addChannel(t, "live", "default", "alpha", 0, 1, common.ChannelStatusEnabled)
	selected, err := GetSatisfiedChannel("default", "alpha")
	if err != nil || selected.ID != live.ID {
		t.Fatalf("disabled channels hid the live candidate: %+v %v", selected, err)
	}
}

func TestChannelResourceGateSurvivesReenableAndDelete(t *testing.T) {
	newTestDB(t)
	ch := addChannel(t, "model gates", "default", "alpha,beta", 0, 1, common.ChannelStatusEnabled)
	if _, err := SetChannelResourceEnabled(ch.ID, "alpha", false); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func() error{
		func() error { return UpdateChannelStatus(ch.ID, common.ChannelStatusEnabled) },
		func() error { return SetAbilitiesEnabledByChannel(ch.ID, true) },
		func() error { return RebuildAbilities(ch) },
		func() error { return DB.Model(&Ability{}).Where("channel_id = ?", ch.ID).Update("enabled", true).Error },
	} {
		if err := mutate(); err != nil {
			t.Fatal(err)
		}
		if _, err := GetSatisfiedChannel("default", "alpha"); !errors.Is(err, ErrNoChannel) {
			t.Fatal("reenable bypassed model gate", err)
		}
		models, err := ListModelsByGroup("default")
		if err != nil || slices.Contains(models, "alpha") || !slices.Contains(models, "beta") {
			t.Fatalf("discovery: %v %v", models, err)
		}
		if enabled, err := ChannelHasEnabledAbility(t.Context(), ch.ID, "default", "alpha"); err != nil || enabled {
			t.Fatal("historical access bypassed model gate", err)
		}
	}
	if err := DeleteChannel(ch.ID); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := DB.Model(&ChannelResourceDisable{}).Where("channel_id = ?", ch.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("deleted channel retained operational gates", err)
	}
}

func TestChannelResourceGateFiltersScoredRouting(t *testing.T) {
	channels, _ := scoredFixture(t, 4)
	for _, ch := range channels[:3] {
		if err := DB.Create(&ChannelResourceDisable{ChannelID: ch.ID, Model: "test-model"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	selected, err := GetScoredChannel("default", "test-model", nil, 0, "", true, nil)
	if err != nil || selected.ID != channels[3].ID {
		t.Fatalf("scored routing ignored model gates: %+v %v", selected, err)
	}
}

func TestManagedChannelResourceGatePreservesPublication(t *testing.T) {
	ch, _, bindings := publicationFixture(t)
	catalog, err := ReadResourceCatalog(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	snapshot := *catalog.Snapshot
	snapshot.Sequence++
	snapshot.Instances[0].UpstreamModel = "upstream-alpha"
	catalog, err = SaveResourceSnapshot(t.Context(), catalog.Revision, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	name := "alpha"
	if _, err := SaveResourcePlacement(t.Context(), catalog.Revision, "resource-1", ResourcePlacement{ModelID: &name, GroupIDs: []string{"default"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	plan := publicationPlan(t, bindings)
	if !plan.Ready {
		t.Fatalf("publication fixture is not ready: %+v", plan)
	}
	if _, err := PublishResourceGateway(t.Context(), publicationExpected(&plan.ResourcePublicationContext), bindings, plan.ReviewDigest, 1); err != nil {
		t.Fatal(err)
	}
	var before ResourceGatewayManagedChannel
	DB.First(&before, "channel_id = ?", ch.ID)
	beforeCatalog, _ := ReadResourceCatalog(t.Context())
	for _, enabled := range []bool{false, true} {
		if _, err := SetChannelResourceEnabled(ch.ID, "alpha", enabled); err != nil {
			t.Fatal(err)
		}
		policy, managed, err := ReadResourceGatewayPolicy(t.Context(), ch.ID, "default", "alpha")
		if err != nil || !managed || (policy != nil) != enabled {
			t.Fatalf("policy: %+v %v", policy, err)
		}
		if enabled && policy.UpstreamModel() != "upstream-alpha" {
			t.Fatal("model alias changed")
		}
		if other, _, err := ReadResourceGatewayPolicy(t.Context(), ch.ID, "vip", "beta"); err != nil || other == nil {
			t.Fatal("other resource was revoked", err)
		}
		var after ResourceGatewayManagedChannel
		DB.First(&after, "channel_id = ?", ch.ID)
		afterCatalog, _ := ReadResourceCatalog(t.Context())
		if !reflect.DeepEqual(before, after) || beforeCatalog.Revision != afterCatalog.Revision {
			t.Fatal("operational toggle changed publication or inventory")
		}
	}
	if _, err := SetChannelResourceEnabled(ch.ID, "upstream-alpha", false); !errors.Is(err, ErrChannelResourceNotFound) {
		t.Fatal("upstream name bypassed published alias", err)
	}
	if _, err := SetChannelResourceEnabled(ch.ID, "alpha", false); err != nil {
		t.Fatal(err)
	}
	plan = publicationPlan(t, bindings)
	if _, err := PublishResourceGateway(t.Context(), publicationExpected(&plan.ResourcePublicationContext), bindings, plan.ReviewDigest, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := GetSatisfiedChannel("default", "alpha"); !errors.Is(err, ErrNoChannel) {
		t.Fatal("publication restored disabled resource", err)
	}
	if err := revokeResourceGatewayChannel(DB, ch.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := SetChannelResourceEnabled(ch.ID, "alpha", true); !errors.Is(err, ErrChannelResourceNotFound) {
		t.Fatal("single enable restored revoked grants", err)
	}
	if _, err := SetChannelResourcesEnabled(ch.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := GetSatisfiedChannel("default", "alpha"); !errors.Is(err, ErrNoChannel) {
		t.Fatal("bulk enable restored revoked grants", err)
	}
}

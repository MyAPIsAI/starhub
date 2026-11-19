package model

import (
	"errors"
	"testing"

	"starhub/common"
)

func TestSetChannelResourceEnabledPersistsAndRebuilds(t *testing.T) {
	newTestDB(t)
	ch := addChannel(t, "single key", "default,vip", "alpha,beta", 0, 1, common.ChannelStatusEnabled)
	other := addChannel(t, "other", "default", "other", 0, 1, common.ChannelStatusEnabled)
	if _, err := SetChannelResourceEnabled(ch.ID, "alpha", false); err != nil {
		t.Fatal(err)
	}
	if _, err := GetSatisfiedChannel("default", "alpha"); !errors.Is(err, ErrNoChannel) {
		t.Fatal("closed resource still routes", err)
	}
	if _, err := GetSatisfiedChannel("default", "beta"); err != nil {
		t.Fatal("unrelated resource closed", err)
	}
	stored, _ := GetChannelByID(ch.ID)
	if err := UpdateChannel(stored); err != nil {
		t.Fatal(err)
	}
	if _, err := GetSatisfiedChannel("vip", "alpha"); !errors.Is(err, ErrNoChannel) {
		t.Fatal("ordinary edit restored disabled resource", err)
	}
	if _, err := SetChannelResourceEnabled(ch.ID, "missing", true); !errors.Is(err, ErrChannelResourceNotFound) {
		t.Fatal(err)
	}
	if _, err := SetChannelResourcesEnabled(ch.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := SetChannelResourceEnabled(ch.ID, "alpha", true); !errors.Is(err, ErrChannelResourceUnavailable) {
		t.Fatal("single enable opened closed channel", err)
	}
	if _, err := SetChannelResourcesEnabled(ch.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := GetSatisfiedChannel("default", "alpha"); err != nil {
		t.Fatal("enable all did not restore model", err)
	}
	if selected, err := GetSatisfiedChannel("default", "other"); err != nil || selected.ID != other.ID {
		t.Fatal("unrelated channel changed", err)
	}
}

func TestChannelResourceStateCountsModelsInsteadOfKeys(t *testing.T) {
	newTestDB(t)
	ch := addChannel(t, "resources", "default,vip", "alpha,beta,alpha", 0, 1, common.ChannelStatusEnabled)
	ch.Key = "first\nsecond\nthird"
	ch.MultiKeyStatus = "2,1,2"
	if err := UpdateChannel(ch); err != nil {
		t.Fatal(err)
	}
	check := func(state string, enabled int) {
		t.Helper()
		states, err := ReadChannelResourceStates([]Channel{*ch})
		if err != nil || states[ch.ID].State != state || states[ch.ID].Total != 2 || states[ch.ID].Enabled != enabled {
			t.Fatalf("resources: %+v %v", states, err)
		}
	}
	check("enabled", 2)
	if err := DB.Create(&ChannelResourceDisable{ChannelID: ch.ID, Model: "alpha"}).Error; err != nil {
		t.Fatal(err)
	}
	check("partial", 1)
	ch.Status = common.ChannelStatusManuallyDisabled
	check("closed", 0)
	ch.Status = common.ChannelStatusEnabled
	if err := DB.Create(&ChannelResourceDisable{ChannelID: ch.ID, Model: "beta"}).Error; err != nil {
		t.Fatal(err)
	}
	check("closed", 0)
	if err := DB.Migrator().DropTable(&ChannelResourceDisable{}); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadChannelResourceStates([]Channel{*ch}); err == nil {
		t.Fatal("resource read failure silently reported enabled")
	}
}

func TestChannelResourceStateUsesPublishedModelAliases(t *testing.T) {
	ch, _, bindings := publicationFixture(t)
	catalog, err := ReadResourceCatalog(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	name := "alpha"
	if _, err := SaveResourcePlacement(t.Context(), catalog.Revision, "resource-1", ResourcePlacement{ModelID: &name, GroupIDs: []string{"default", "vip"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	plan := publicationPlan(t, bindings)
	if _, err := PublishResourceGateway(t.Context(), publicationExpected(&plan.ResourcePublicationContext), bindings, plan.ReviewDigest, 1); err != nil {
		t.Fatal(err)
	}
	ch.Models = "unpublished"
	states, err := ReadChannelResourceStates([]Channel{*ch})
	if err != nil || states[ch.ID].Total != 2 || states[ch.ID].Resources[0].Model != "alpha" {
		t.Fatalf("published resources: %+v %v", states, err)
	}
	if err := DB.Create(&ChannelResourceDisable{ChannelID: ch.ID, Model: "alpha"}).Error; err != nil {
		t.Fatal(err)
	}
	states, err = ReadChannelResourceStates([]Channel{*ch})
	if err != nil || states[ch.ID].State != "partial" {
		t.Fatalf("published partial state: %+v %v", states, err)
	}
	if err := revokeResourceGatewayChannel(DB, ch.ID); err != nil {
		t.Fatal(err)
	}
	states, err = ReadChannelResourceStates([]Channel{*ch})
	if err != nil || states[ch.ID].State != "closed" || states[ch.ID].Total != 0 {
		t.Fatalf("revoked resources: %+v %v", states, err)
	}
}

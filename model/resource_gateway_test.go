package model

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"starhub/common"
)

func managedGatewayFixture(t *testing.T, ch *Channel, active bool, pairs ...ResourceGatewayRoute) {
	t.Helper()
	if err := DB.Create(&ResourceGatewayManagedChannel{ChannelID: ch.ID, SourceChannelID: fmt.Sprintf("source-%d", ch.ID), Active: active, Profile: ResourceGatewayTextProfile}).Error; err != nil {
		t.Fatal(err)
	}
	for i := range pairs {
		pairs[i].ChannelID = ch.ID
		pairs[i].Capabilities = `["code","text"]`
		pairs[i].ContextLength = 128000
	}
	if len(pairs) > 0 {
		if err := DB.Create(&pairs).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func assertGatewayPairs(t *testing.T, channelID int64, active bool) {
	t.Helper()
	for _, entry := range []struct {
		group, model string
		allowed      bool
	}{{"default", "alpha", true}, {"vip", "beta", true}, {"vip", "alpha", false}, {"default", "beta", false}, {"default", "injected", false}, {"other", "injected", false}} {
		for _, selectChannel := range []func() (*Channel, error){func() (*Channel, error) { return GetSatisfiedChannel(entry.group, entry.model) }, func() (*Channel, error) {
			return GetSatisfiedChannelPreferred(entry.group, entry.model, nil, channelID)
		}, func() (*Channel, error) { return GetSatisfiedChannelHashed(entry.group, entry.model, nil, "sticky") }} {
			ch, err := selectChannel()
			if active && entry.allowed {
				if err != nil || ch.ID != channelID {
					t.Fatalf("missing %s/%s: %+v %v", entry.group, entry.model, ch, err)
				}
			} else if !errors.Is(err, ErrNoChannel) {
				t.Fatalf("unauthorized %s/%s: %+v %v", entry.group, entry.model, ch, err)
			}
		}
	}
	for group, model := range map[string]string{"default": "alpha", "vip": "beta"} {
		models, err := ListModelsByGroup(group)
		if err != nil {
			t.Fatal(err)
		}
		if active {
			if !reflect.DeepEqual(models, []string{model}) {
				t.Fatalf("discovery exposed extra pairs in %s: %v", group, models)
			}
		} else if len(models) != 0 {
			t.Fatalf("revoked discovery in %s: %v", group, models)
		}
	}
	coverage, err := ListModelCoverage()
	if err != nil {
		t.Fatal(err)
	}
	names, err := AllPlazaModels()
	if err != nil {
		t.Fatal(err)
	}
	if active {
		if len(coverage) != 2 || !reflect.DeepEqual(coverage["alpha"].Groups, []string{"default"}) || !reflect.DeepEqual(coverage["beta"].Groups, []string{"vip"}) || !reflect.DeepEqual(names, []string{"alpha", "beta"}) {
			t.Fatalf("plaza broadened pairs: %+v %v", coverage, names)
		}
	} else if len(coverage) != 0 || len(names) != 0 {
		t.Fatalf("plaza exposed revoked routes: %+v %v", coverage, names)
	}
}

func TestResourceGatewayExactPairsSurviveManualAbilityRebuilds(t *testing.T) {
	newTestDB(t)
	ch := addChannel(t, "managed", "default,vip", "alpha,beta", 0, 1, common.ChannelStatusEnabled)
	managedGatewayFixture(t, ch, true, ResourceGatewayRoute{Model: "alpha", Group: "default"}, ResourceGatewayRoute{Model: "beta", Group: "vip"})
	// Existing cross-product rows represent stale or independently enabled abilities.
	assertGatewayPairs(t, ch.ID, true)
	if err := RebuildAbilities(ch); err != nil {
		t.Fatal(err)
	}
	var rows []Ability
	if err := DB.Where("channel_id = ?", ch.ID).Find(&rows).Error; err != nil || len(rows) != 2 {
		t.Fatalf("rebuild did not use exact grants: %+v %v", rows, err)
	}
	ch.Models = "alpha,beta,injected"
	ch.Group = "default,vip,other"
	if err := UpdateChannel(ch); err != nil {
		t.Fatal(err)
	}
	if err := DB.Create(&Ability{ChannelID: ch.ID, Model: "injected", Group: "other", Enabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := SetAbilitiesEnabledByChannel(ch.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := UpdateChannelStatus(ch.ID, common.ChannelStatusManuallyDisabled); err != nil {
		t.Fatal(err)
	}
	if err := UpdateChannelStatus(ch.ID, common.ChannelStatusEnabled); err != nil {
		t.Fatal(err)
	}
	assertGatewayPairs(t, ch.ID, true)
	if err := DeleteAbilitiesByChannel(ch.ID); err != nil {
		t.Fatal(err)
	}
	if err := AddAbilities(ch); err != nil {
		t.Fatal(err)
	}
	assertGatewayPairs(t, ch.ID, true)
}

func TestResourceGatewayRevokedMarkerCannotBeReopenedByManualEdits(t *testing.T) {
	newTestDB(t)
	ch := addChannel(t, "revoked", "default,vip", "alpha,beta", 0, 1, common.ChannelStatusEnabled)
	managedGatewayFixture(t, ch, false, ResourceGatewayRoute{Model: "alpha", Group: "default"}, ResourceGatewayRoute{Model: "beta", Group: "vip"})
	assertGatewayPairs(t, ch.ID, false)
	if err := SetAbilitiesEnabledByChannel(ch.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := UpdateChannelStatus(ch.ID, common.ChannelStatusEnabled); err != nil {
		t.Fatal(err)
	}
	assertGatewayPairs(t, ch.ID, false)
	if err := RebuildAbilities(ch); err != nil {
		t.Fatal(err)
	}
	ch.Models = "alpha,beta,injected"
	ch.Group = "default,vip,other"
	if err := UpdateChannel(ch); err != nil {
		t.Fatal(err)
	}
	assertGatewayPairs(t, ch.ID, false)
}

func TestResourceGatewayDeletionRetainsRevocationAcrossRestartAndRecreation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "managed-gateway.db")
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	ch := addChannel(t, "managed", "default,vip", "alpha,beta", 0, 1, common.ChannelStatusEnabled)
	managedGatewayFixture(t, ch, true, ResourceGatewayRoute{Model: "alpha", Group: "default"}, ResourceGatewayRoute{Model: "beta", Group: "vip"})
	if err := DeleteChannel(ch.ID); err != nil {
		t.Fatal(err)
	}
	var record ResourceGatewayManagedChannel
	if err := DB.First(&record, "channel_id = ?", ch.ID).Error; err != nil || record.Active {
		t.Fatalf("deletion removed ownership marker: %+v %v", record, err)
	}
	var count int64
	if err := DB.Model(&ResourceGatewayRoute{}).Where("channel_id = ?", ch.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("deletion retained grants: %d %v", count, err)
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	if err := InsertChannel(ch); err != nil {
		t.Fatal(err)
	}
	assertGatewayPairs(t, ch.ID, false)
	legacy := addChannel(t, "legacy", "default", "injected", 0, 1, common.ChannelStatusEnabled)
	if got, err := GetSatisfiedChannel("default", "injected"); err != nil || got.ID != legacy.ID {
		t.Fatalf("unmanaged legacy channel changed: %+v %v", got, err)
	}
}

func TestResourceGatewayRestrictionsPrecedeCandidateLimit(t *testing.T) {
	newTestDB(t)
	for i := 0; i < 65; i++ {
		ch := addChannel(t, fmt.Sprintf("blocked-%02d", i), "default", "alpha", 10, 1, common.ChannelStatusEnabled)
		managedGatewayFixture(t, ch, true, ResourceGatewayRoute{Model: "beta", Group: "vip"})
	}
	legacy := addChannel(t, "allowed", "default", "alpha", 0, 1, common.ChannelStatusEnabled)
	for _, pick := range []func() (*Channel, error){func() (*Channel, error) { return GetSatisfiedChannel("default", "alpha") }, func() (*Channel, error) { return GetSatisfiedChannelPreferred("default", "alpha", nil, 1) }, func() (*Channel, error) { return GetSatisfiedChannelHashed("default", "alpha", nil, "hash") }} {
		got, err := pick()
		if err != nil || got.ID != legacy.ID {
			t.Fatalf("blocked candidates hid valid channel after limit64: %+v %v", got, err)
		}
	}
}

func TestResourceGatewayMissingOwnershipTablesFailClosed(t *testing.T) {
	for _, table := range []any{&ResourceGatewayManagedChannel{}, &ResourceGatewayRoute{}} {
		t.Run(fmt.Sprintf("%T", table), func(t *testing.T) {
			newTestDB(t)
			ch := addChannel(t, "unmanaged", "default", "alpha", 0, 1, common.ChannelStatusEnabled)
			if err := DB.Migrator().DropTable(table); err != nil {
				t.Fatal(err)
			}
			if _, err := GetSatisfiedChannel("default", "alpha"); err == nil {
				t.Fatal("selection ignored missing ownership state")
			}
			if _, err := ListModelsByGroup("default"); err == nil {
				t.Fatal("model discovery ignored missing ownership state")
			}
			if _, err := ListModelCoverage(); err == nil {
				t.Fatal("coverage ignored missing ownership state")
			}
			if _, err := AllPlazaModels(); err == nil {
				t.Fatal("plaza ignored missing ownership state")
			}
			// Channel deletion must roll back if it cannot retain/revoke ownership safely.
			before := channelRevision(t)
			if err := DeleteChannel(ch.ID); err == nil {
				t.Fatal("deletion succeeded without durable ownership state")
			}
			if _, err := GetChannelByID(ch.ID); err != nil || channelRevision(t) != before {
				t.Fatalf("failed deletion did not roll back: %v", err)
			}
		})
	}
}

package model

import (
	"errors"
	"reflect"
	"testing"

	"starhub/common"
)

func TestSetChannelResourcesEnabledPersistsAvailabilityOnly(t *testing.T) {
	newTestDB(t)
	ch := addChannel(t, "multi", "default,vip", "alpha,beta", 4, 7, common.ChannelStatusEnabled)
	ch.Key = "first-key\nsecond-key"
	ch.MultiKeyStatus = "2,1"
	ch.KeyMode = KeyModeRoundRobin
	ch.BaseURL = "https://example.test"
	if err := UpdateChannel(ch); err != nil {
		t.Fatal(err)
	}
	before, _ := GetChannelByID(ch.ID)
	other := addChannel(t, "unrelated", "default", "other", 0, 1, common.ChannelStatusEnabled)
	var revision ChannelConfigurationState
	DB.First(&revision, 1)
	for _, enabled := range []bool{false, true} {
		returned, err := SetChannelResourcesEnabled(ch.ID, enabled)
		if err != nil {
			t.Fatal(err)
		}
		stored, err := GetChannelByID(ch.ID)
		if err != nil || !reflect.DeepEqual(returned, stored) {
			t.Fatalf("response does not match stored channel: %v", err)
		}
		want := *before
		want.Status = common.ChannelStatusManuallyDisabled
		if enabled {
			want.Status = common.ChannelStatusEnabled
			want.MultiKeyStatus = ""
			if !reflect.DeepEqual(stored.KeyStatuses(), []int{1, 1}) {
				t.Fatal("explicit enable did not restore all resources")
			}
		}
		if !reflect.DeepEqual(stored, &want) {
			t.Fatal("availability update changed unrelated channel configuration")
		}
		for _, group := range []string{"default", "vip"} {
			for _, name := range []string{"alpha", "beta"} {
				selected, err := GetSatisfiedChannel(group, name)
				if enabled && (err != nil || selected.ID != ch.ID) || !enabled && !errors.Is(err, ErrNoChannel) {
					t.Fatalf("routing disagrees with availability %t: %v", enabled, err)
				}
			}
		}
	}
	var afterRevision ChannelConfigurationState
	DB.First(&afterRevision, 1)
	if afterRevision.Revision != revision.Revision+2 {
		t.Fatal("availability changes did not advance the shared configuration revision")
	}
	unchanged, _ := GetChannelByID(other.ID)
	if !reflect.DeepEqual(unchanged, other) {
		t.Fatal("availability change affected a different channel")
	}
}

func TestSetChannelResourcesEnabledPreservesManagedGrants(t *testing.T) {
	newTestDB(t)
	ch := addChannel(t, "managed", "default,vip", "alpha,beta", 0, 1, common.ChannelStatusEnabled)
	managedGatewayFixture(t, ch, true, ResourceGatewayRoute{Model: "alpha", Group: "default"}, ResourceGatewayRoute{Model: "beta", Group: "vip"})
	if _, err := SetChannelResourcesEnabled(ch.ID, false); err != nil {
		t.Fatal(err)
	}
	assertGatewayPairs(t, ch.ID, false)
	if _, err := SetChannelResourcesEnabled(ch.ID, true); err != nil {
		t.Fatal(err)
	}
	assertGatewayPairs(t, ch.ID, true)
	if err := DB.Model(&ResourceGatewayManagedChannel{}).Where("channel_id = ?", ch.ID).Update("active", false).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := SetChannelResourcesEnabled(ch.ID, true); err != nil {
		t.Fatal(err)
	}
	assertGatewayPairs(t, ch.ID, false)
}

func TestSetChannelResourcesEnabledRollsBackInvalidConfiguration(t *testing.T) {
	newTestDB(t)
	ch := addChannel(t, "invalid-plugin", "default", "alpha", 0, 1, common.ChannelStatusManuallyDisabled)
	if err := DB.Model(&Channel{}).Where("id = ?", ch.ID).Updates(map[string]any{"task_plugin_id": 999, "multi_key_status": "2"}).Error; err != nil {
		t.Fatal(err)
	}
	before, _ := GetChannelByID(ch.ID)
	var revision ChannelConfigurationState
	DB.First(&revision, 1)
	if _, err := SetChannelResourcesEnabled(ch.ID, true); !errors.Is(err, ErrTaskPluginConfiguration) {
		t.Fatalf("invalid plugin configuration was enabled: %v", err)
	}
	after, _ := GetChannelByID(ch.ID)
	var afterRevision ChannelConfigurationState
	DB.First(&afterRevision, 1)
	if !reflect.DeepEqual(before, after) || revision != afterRevision {
		t.Fatal("rejected availability change was not rolled back")
	}
	if _, err := SetChannelResourcesEnabled(ch.ID, false); err != nil {
		t.Fatal("closing must remain available for an invalid plugin binding")
	}
	if _, err := SetChannelResourcesEnabled(99999, true); !errors.Is(err, ErrChannelNotFound) {
		t.Fatalf("missing channel: %v", err)
	}
}

package model

import (
	"testing"

	"starhub/common"
)

func TestUpdateChannelStatusTogglesAbilities(t *testing.T) {
	newTestDB(t)
	ch := addChannel(t, "toggle", "default", "gpt-4o", 0, 1, common.ChannelStatusEnabled)

	if err := UpdateChannelStatus(ch.ID, common.ChannelStatusManuallyDisabled); err != nil {
		t.Fatalf("UpdateChannelStatus(disable) failed: %v", err)
	}
	var disabled int64
	if err := DB.Model(&Ability{}).Where("channel_id = ? AND enabled = ?", ch.ID, false).
		Count(&disabled).Error; err != nil {
		t.Fatalf("count failed: %v", err)
	}
	if disabled == 0 {
		t.Fatal("abilities were not disabled along with the channel")
	}
	if _, err := GetSatisfiedChannel("default", "gpt-4o"); err != ErrNoChannel {
		t.Fatalf("disabled channel still selectable: %v", err)
	}

	if err := UpdateChannelStatus(ch.ID, common.ChannelStatusEnabled); err != nil {
		t.Fatalf("UpdateChannelStatus(enable) failed: %v", err)
	}
	got, err := GetSatisfiedChannel("default", "gpt-4o")
	if err != nil {
		t.Fatalf("re-enabled channel not selectable: %v", err)
	}
	if got.ID != ch.ID {
		t.Fatalf("got channel %d, want %d", got.ID, ch.ID)
	}
}

func TestUpdateChannelRollsBackWhenAbilitiesAreInvalid(t *testing.T) {
	newTestDB(t)
	channel := addChannel(t, "stable", "default", "gpt-4o", 0, 1, common.ChannelStatusEnabled)
	channel.Models = ""
	if err := UpdateChannel(channel); err == nil {
		t.Fatal("invalid channel update should fail")
	}
	stored, err := GetChannelByID(channel.ID)
	if err != nil || stored.Models != "gpt-4o" {
		t.Fatalf("channel changed despite failed update: %+v, %v", stored, err)
	}
	if _, err := GetSatisfiedChannel("default", "gpt-4o"); err != nil {
		t.Fatalf("ability disappeared despite failed update: %v", err)
	}
}

func TestUpdateChannelStatusNotFound(t *testing.T) {
	newTestDB(t)
	if err := UpdateChannelStatus(9999, common.ChannelStatusEnabled); err != ErrChannelNotFound {
		t.Fatalf("want ErrChannelNotFound, got %v", err)
	}
}

func TestGetChannelByIDRejectsNonPositive(t *testing.T) {
	newTestDB(t)
	for _, id := range []int64{0, -1} {
		if _, err := GetChannelByID(id); err != ErrChannelNotFound {
			t.Fatalf("GetChannelByID(%d) = %v, want ErrChannelNotFound", id, err)
		}
	}
}

func TestDeleteChannelNotFound(t *testing.T) {
	newTestDB(t)
	if err := DeleteChannel(9999); err != ErrChannelNotFound {
		t.Fatalf("want ErrChannelNotFound, got %v", err)
	}
}

// Deleting a channel must take its ability rows with it, otherwise the
// selector would keep handing out a channel that no longer exists.
func TestDeleteChannelRemovesAbilities(t *testing.T) {
	newTestDB(t)
	ch := addChannel(t, "doomed", "default", "gpt-4o", 0, 1, common.ChannelStatusEnabled)

	if err := DeleteChannel(ch.ID); err != nil {
		t.Fatalf("DeleteChannel failed: %v", err)
	}
	var count int64
	if err := DB.Model(&Ability{}).Where("channel_id = ?", ch.ID).Count(&count).Error; err != nil {
		t.Fatalf("count failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("ability rows left behind: %d", count)
	}
	if _, err := GetSatisfiedChannel("default", "gpt-4o"); err != ErrNoChannel {
		t.Fatalf("want ErrNoChannel after delete, got %v", err)
	}
}

func TestUpdateChannelRebuildsAbilities(t *testing.T) {
	newTestDB(t)
	ch := addChannel(t, "grow", "default", "gpt-4o", 0, 1, common.ChannelStatusEnabled)

	ch.Models = "gpt-4o, claude-3"
	if err := UpdateChannel(ch); err != nil {
		t.Fatalf("UpdateChannel failed: %v", err)
	}
	var count int64
	if err := DB.Model(&Ability{}).Where("channel_id = ?", ch.ID).Count(&count).Error; err != nil {
		t.Fatalf("count failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("ability rows = %d, want 2 after adding a model", count)
	}
	if _, err := GetSatisfiedChannel("default", "claude-3"); err != nil {
		t.Fatalf("newly added model not selectable: %v", err)
	}
}

func TestDisableChannelKeyPersistsAndReportsAllDisabled(t *testing.T) {
	newTestDB(t)
	ch := &Channel{
		Name:   "multi",
		Type:   common.DefaultChannelType,
		Key:    "k1\nk2",
		Models: "gpt-4o",
		Group:  "default",
		Status: common.ChannelStatusEnabled,
		Weight: 1,
	}
	if err := InsertChannel(ch); err != nil {
		t.Fatalf("InsertChannel failed: %v", err)
	}

	allDisabled, err := DisableChannelKey(ch.ID, 0)
	if err != nil {
		t.Fatalf("DisableChannelKey(0) failed: %v", err)
	}
	if allDisabled {
		t.Fatal("disabling one of two keys should not report all-disabled")
	}
	stored, err := GetChannelByID(ch.ID)
	if err != nil {
		t.Fatalf("GetChannelByID failed: %v", err)
	}
	statuses := stored.KeyStatuses()
	if statuses[0] != common.StatusDisabled || statuses[1] != common.StatusEnabled {
		t.Fatalf("persisted statuses = %v, want [2 1]", statuses)
	}

	allDisabled, err = DisableChannelKey(ch.ID, 1)
	if err != nil {
		t.Fatalf("DisableChannelKey(1) failed: %v", err)
	}
	if !allDisabled {
		t.Fatal("disabling the last enabled key should report all-disabled")
	}
}

// An out-of-range index is a no-op rather than an error, and must not corrupt
// the stored status list.
func TestDisableChannelKeyOutOfRangeIsNoop(t *testing.T) {
	newTestDB(t)
	ch := &Channel{
		Name: "multi", Type: common.DefaultChannelType, Key: "k1\nk2",
		Models: "gpt-4o", Group: "default", Status: common.ChannelStatusEnabled, Weight: 1,
	}
	if err := InsertChannel(ch); err != nil {
		t.Fatalf("InsertChannel failed: %v", err)
	}

	for _, idx := range []int{-1, 5} {
		allDisabled, err := DisableChannelKey(ch.ID, idx)
		if err != nil {
			t.Fatalf("DisableChannelKey(%d) failed: %v", idx, err)
		}
		if allDisabled {
			t.Fatalf("DisableChannelKey(%d) must not report all-disabled", idx)
		}
	}
	stored, err := GetChannelByID(ch.ID)
	if err != nil {
		t.Fatalf("GetChannelByID failed: %v", err)
	}
	if stored.MultiKeyStatus != "" {
		t.Fatalf("out-of-range disable wrote %q, want untouched", stored.MultiKeyStatus)
	}
}

func TestDisableChannelKeyNotFound(t *testing.T) {
	newTestDB(t)
	if _, err := DisableChannelKey(9999, 0); err != ErrChannelNotFound {
		t.Fatalf("want ErrChannelNotFound, got %v", err)
	}
}

func TestIncreaseChannelUsedQuotaIgnoresNonPositive(t *testing.T) {
	newTestDB(t)
	ch := addChannel(t, "quota", "default", "gpt-4o", 0, 1, common.ChannelStatusEnabled)

	for _, amount := range []int64{0, -100} {
		if err := IncreaseChannelUsedQuota(ch.ID, amount); err != nil {
			t.Fatalf("IncreaseChannelUsedQuota(%d) failed: %v", amount, err)
		}
	}
	got, err := GetChannelByID(ch.ID)
	if err != nil {
		t.Fatalf("GetChannelByID failed: %v", err)
	}
	if got.UsedQuota != 0 {
		t.Fatalf("used_quota = %d, want 0", got.UsedQuota)
	}

	if err := IncreaseChannelUsedQuota(ch.ID, 500); err != nil {
		t.Fatalf("IncreaseChannelUsedQuota(500) failed: %v", err)
	}
	if err := IncreaseChannelUsedQuota(ch.ID, 250); err != nil {
		t.Fatalf("IncreaseChannelUsedQuota(250) failed: %v", err)
	}
	got, _ = GetChannelByID(ch.ID)
	if got.UsedQuota != 750 {
		t.Fatalf("used_quota = %d, want 750 (accumulating)", got.UsedQuota)
	}
}

func TestUpdateChannelResponseTime(t *testing.T) {
	newTestDB(t)
	ch := addChannel(t, "rt", "default", "gpt-4o", 0, 1, common.ChannelStatusEnabled)

	if err := UpdateChannelResponseTime(ch.ID, 321); err != nil {
		t.Fatalf("UpdateChannelResponseTime failed: %v", err)
	}
	got, _ := GetChannelByID(ch.ID)
	if got.ResponseTime != 321 {
		t.Fatalf("response_time = %d, want 321", got.ResponseTime)
	}

	if err := UpdateChannelResponseTime(9999, 1); err != ErrChannelNotFound {
		t.Fatalf("want ErrChannelNotFound, got %v", err)
	}
}

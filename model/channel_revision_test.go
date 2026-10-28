package model

import (
	"errors"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"starhub/common"
)

func channelRevision(t *testing.T) int64 {
	t.Helper()
	var state ChannelConfigurationState
	if err := DB.First(&state, 1).Error; err != nil {
		t.Fatal(err)
	}
	return state.Revision
}

func TestChannelRevisionCoversConfigurationButNotTraffic(t *testing.T) {
	newTestDB(t)
	ch := &Channel{Name: "original", Key: "one\ntwo", Models: "model", Group: "default", Status: 1}
	operations := []func() error{
		func() error { return InsertChannel(ch) },
		func() error { ch.Name = "changed"; return UpdateChannel(ch) },
		func() error { ch.Name = "original"; return UpdateChannel(ch) },
		func() error { return UpdateChannelStatus(ch.ID, common.ChannelStatusManuallyDisabled) },
		func() error { return UpdateChannelStatus(ch.ID, common.ChannelStatusEnabled) },
		func() error { _, err := DisableChannelKey(ch.ID, 0); return err },
		func() error { return RebuildAbilities(ch) },
		func() error { return SetAbilitiesEnabledByChannel(ch.ID, false) },
		func() error { return DeleteAbilitiesByChannel(ch.ID) },
		func() error { return AddAbilities(ch) },
		func() error { return DeleteChannel(ch.ID) },
		func() error { return InsertChannel(ch) },
	}
	for i, operation := range operations {
		if err := operation(); err != nil {
			t.Fatalf("operation %d: %v", i, err)
		}
		if got := channelRevision(t); got != int64(i+1) {
			t.Fatalf("operation %d revision: %d", i, got)
		}
	}
	before := channelRevision(t)
	if err := UpdateChannelResponseTime(ch.ID, 200); err != nil {
		t.Fatal(err)
	}
	if err := IncreaseChannelUsedQuota(ch.ID, 100); err != nil {
		t.Fatal(err)
	}
	if channelRevision(t) != before {
		t.Fatal("traffic invalidated a configuration preview")
	}
}

func TestChannelRevisionRollbackAndFailClosed(t *testing.T) {
	newTestDB(t)
	ch := addChannel(t, "stable", "default", "model", 0, 1, common.ChannelStatusEnabled)
	before := channelRevision(t)
	ch.Models = ""
	if err := UpdateChannel(ch); err == nil || channelRevision(t) != before {
		t.Fatalf("failed update advanced revision: %v", err)
	}
	if err := DeleteChannel(999999); !errors.Is(err, ErrChannelNotFound) || channelRevision(t) != before {
		t.Fatalf("failed delete advanced revision: %v", err)
	}
	for _, revision := range []int64{-1, maxResourceRevision} {
		if err := DB.Model(&ChannelConfigurationState{}).Where("id = 1").UpdateColumn("revision", revision).Error; err != nil {
			t.Fatal(err)
		}
		if err := UpdateChannelStatus(ch.ID, common.ChannelStatusManuallyDisabled); !errors.Is(err, ErrChannelRevisionUnavailable) {
			t.Fatalf("unsafe revision %d permitted mutation: %v", revision, err)
		}
		stored, err := GetChannelByID(ch.ID)
		if err != nil || stored.Status != common.ChannelStatusEnabled {
			t.Fatalf("unsafe revision changed channel: %+v %v", stored, err)
		}
	}
	if err := DB.Delete(&ChannelConfigurationState{}, 1).Error; err != nil {
		t.Fatal(err)
	}
	if err := DeleteChannel(ch.ID); !errors.Is(err, ErrChannelRevisionUnavailable) {
		t.Fatalf("missing revision permitted deletion: %v", err)
	}
}

func TestChannelRevisionAndKeyDisablesSerializeAcrossWriters(t *testing.T) {
	newTestDB(t)
	ch := &Channel{Name: "keys", Key: "a\nb\nc\nd\ne\nf\ng\nh", Models: "model", Group: "default", Status: 1}
	if err := InsertChannel(ch); err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	errors := make(chan error, 8)
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func(index int) {
			defer workers.Done()
			_, err := DisableChannelKey(ch.ID, index)
			errors <- err
		}(i)
	}
	workers.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	stored, err := GetChannelByID(ch.ID)
	if err != nil || !reflect.DeepEqual(stored.KeyStatuses(), []int{2, 2, 2, 2, 2, 2, 2, 2}) || channelRevision(t) != 9 {
		t.Fatalf("concurrent disables lost state: %+v %v", stored, err)
	}
}

func TestChannelRevisionSurvivesRestartAndReusedIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "channel-revision.db")
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	ch := &Channel{ID: 42, Name: "same", Models: "model", Group: "default", Status: 1}
	if err := InsertChannel(ch); err != nil {
		t.Fatal(err)
	}
	if err := DeleteChannel(ch.ID); err != nil {
		t.Fatal(err)
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	if channelRevision(t) != 2 {
		t.Fatal("restart reset the revision")
	}
	if err := InsertChannel(ch); err != nil || channelRevision(t) != 3 {
		t.Fatalf("reused identity did not advance: %v", err)
	}
}

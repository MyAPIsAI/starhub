package model

import (
	"errors"
	"testing"

	"starhub/internal/traffic"
	"starhub/internal/upstream"
)

func TestOrdinaryFallbackSwitchAndTrafficOverride(t *testing.T) {
	channels, _ := scoredFixture(t, 2)
	oldPolicy := upstream.PolicyJSON()
	t.Cleanup(func() { _ = upstream.ApplyPolicy(oldPolicy) })
	if err := upstream.ApplyPolicy(`{"ordinary_fallback":true}`); err != nil {
		t.Fatal(err)
	}
	rule := &traffic.Rule{Channels: []traffic.ChannelWeight{{ChannelID: channels[0].ID, Weight: 100}, {ChannelID: channels[1].ID, Weight: 0}}}
	excluded := map[int64]bool{channels[0].ID: true}
	if _, err := GetSatisfiedChannelTraffic("default", "test-model", excluded, 0, "", rule); !errors.Is(err, ErrNoChannel) {
		t.Fatalf("normal routing ignored allocation: %v", err)
	}
	ch, err := GetScoredChannelStage(RoutingOrdinaryFallback, "default", "test-model", excluded, 0, "", true, nil)
	if err != nil || ch.ID != channels[1].ID {
		t.Fatalf("fallback did not bypass allocation: %+v %v", ch, err)
	}
	if err := upstream.ApplyPolicy(`{"ordinary_fallback":false}`); err != nil {
		t.Fatal(err)
	}
	if _, err := GetScoredChannelStage(RoutingOrdinaryFallback, "default", "test-model", excluded, 0, "", true, nil); !errors.Is(err, ErrNoChannel) {
		t.Fatalf("disabled fallback selected channel: %v", err)
	}
}

func TestOrdinaryFallbackRequiresCommercialSnapshot(t *testing.T) {
	channels, _ := scoredFixture(t, 1)
	routingSnapshotState.Lock()
	saved := routingSnapshotState.snapshot
	routingSnapshotState.snapshot = nil
	routingSnapshotState.Unlock()
	t.Cleanup(func() {
		routingSnapshotState.Lock()
		routingSnapshotState.snapshot = saved
		routingSnapshotState.Unlock()
	})
	if allowed, _ := ordinaryPlatformAdmission(channels[0], "test-model"); allowed {
		t.Fatal("commercial fallback admitted before first snapshot")
	}
}

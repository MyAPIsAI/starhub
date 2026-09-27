package controller

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"starhub/common"
	"starhub/model"
	"starhub/relay/dto"
	"starhub/setting"
)

// newAffinityFlow builds a relayFlow the way a relay handler would, without
// opening a socket, so the routing hooks can be exercised in isolation.
func newAffinityFlow(t *testing.T, tokenID int64, group, modelName string) *relayFlow {
	t.Helper()
	req := relayRequest{tokenID: tokenID, group: group, modelName: modelName}
	flow := newRelayFlow(context.Background(), httptest.NewRecorder(), req, dto.InboundOpenAI, "chat")
	t.Cleanup(flow.finish)
	return flow
}

// Affinity is opt-in: only after a successful attempt on an enabled build is
// the channel remembered, and only until a retryable failure clears it.
func TestRelayFlowAffinityStickyAndCleared(t *testing.T) {
	newTestDB(t)
	model.ResetAffinity()
	t.Cleanup(model.ResetAffinity)
	t.Cleanup(setting.ResetAffinityToDefaults)

	// Equal priority: only the sticky preference can make the pick stable.
	mustChannel(t, "aff-a", "gpt-4o", "sk-a", common.ChannelStatusEnabled)
	mustChannel(t, "aff-b", "gpt-4o", "sk-b", common.ChannelStatusEnabled)

	// Disabled by default: success must not create a preference.
	flow := newAffinityFlow(t, 42, common.DefaultGroup, "gpt-4o")
	ch, err := flow.pick()
	if err != nil {
		t.Fatalf("pick failed: %v", err)
	}
	flow.noteAffinity(ch, attemptSuccess)
	if _, ok := model.AffinityChannel(flow.affinityKey()); ok {
		t.Fatal("affinity must stay empty while the feature is disabled")
	}

	// Enabled: the successful channel becomes sticky.
	if err := setting.UpdateOption(setting.OptionKeyRoutingAffinityEnabled, "true"); err != nil {
		t.Fatalf("enable affinity failed: %v", err)
	}
	flow = newAffinityFlow(t, 42, common.DefaultGroup, "gpt-4o")
	ch, err = flow.pick()
	if err != nil {
		t.Fatalf("pick failed: %v", err)
	}
	flow.noteAffinity(ch, attemptSuccess)
	for i := 0; i < 30; i++ {
		next, err := flow.pick()
		if err != nil {
			t.Fatalf("sticky pick %d failed: %v", i, err)
		}
		if next.ID != ch.ID {
			t.Fatalf("sticky pick %d = %d, want the preferred %d", i, next.ID, ch.ID)
		}
	}

	// A retryable failure clears the preference so the next request can move on.
	flow.noteAffinity(ch, attemptUpstreamError)
	if _, ok := model.AffinityChannel(flow.affinityKey()); ok {
		t.Fatal("a failed sticky channel should have been cleared")
	}

	// A client error (4xx) is a no-op: the preference survives.
	flow.noteAffinity(ch, attemptSuccess)
	flow.noteAffinity(ch, attemptClientError)
	if got, ok := model.AffinityChannel(flow.affinityKey()); !ok || got != ch.ID {
		t.Fatalf("client error must not clear the preference: %d, %v", got, ok)
	}

	// Disabling the sticky channel falls back to another channel, no error.
	if err := model.UpdateChannelStatus(ch.ID, common.ChannelStatusManuallyDisabled); err != nil {
		t.Fatalf("disable channel failed: %v", err)
	}
	next, err := flow.pick()
	if err != nil {
		t.Fatalf("pick after disable failed: %v", err)
	}
	if next.ID == ch.ID {
		t.Fatal("a disabled sticky channel was selected")
	}
}

// Without a token there is no stable key to hang a preference on, so affinity
// must be inert even when enabled.
func TestRelayFlowAffinityRequiresToken(t *testing.T) {
	newTestDB(t)
	model.ResetAffinity()
	t.Cleanup(model.ResetAffinity)
	t.Cleanup(setting.ResetAffinityToDefaults)

	mustChannel(t, "aff-a", "gpt-4o", "sk-a", common.ChannelStatusEnabled)
	mustChannel(t, "aff-b", "gpt-4o", "sk-b", common.ChannelStatusEnabled)
	if err := setting.UpdateOption(setting.OptionKeyRoutingAffinityEnabled, "true"); err != nil {
		t.Fatalf("enable affinity failed: %v", err)
	}

	flow := newAffinityFlow(t, 0, common.DefaultGroup, "gpt-4o")
	ch, err := flow.pick()
	if err != nil {
		t.Fatalf("pick failed: %v", err)
	}
	flow.noteAffinity(ch, attemptSuccess)
	if _, ok := model.AffinityChannel(flow.affinityKey()); ok {
		t.Fatal("a token-less request must not create a preference")
	}
}

// A cooling channel must not be honored as the affinity preference, while a
// cleared cooldown restores it.
func TestRelayFlowAffinityCooldownGate(t *testing.T) {
	newTestDB(t)
	model.ResetAffinity()
	t.Cleanup(model.ResetAffinity)
	t.Cleanup(setting.ResetAffinityToDefaults)

	ch := mustChannel(t, "cool", "gpt-4o", "sk-cool", common.ChannelStatusEnabled)
	if err := setting.UpdateOption(setting.OptionKeyRoutingAffinityEnabled, "true"); err != nil {
		t.Fatalf("enable affinity failed: %v", err)
	}

	flow := newAffinityFlow(t, 7, common.DefaultGroup, "gpt-4o")
	model.RememberAffinity(flow.affinityKey(), ch.ID, -1, time.Minute)

	// With the only candidate cooling down, pick still returns it but must not
	// treat it as the sticky preference.
	model.CoolDownChannel(ch.ID, time.Minute)
	next, err := flow.pick()
	if err != nil || next.ID != ch.ID {
		t.Fatalf("pick = %+v, %v; want the only channel", next, err)
	}
	if flow.affinityPreferred == ch.ID {
		t.Fatal("a cooling channel must not be honored as the preference")
	}

	// Clearing the cooldown restores the preference.
	model.CoolDownChannel(ch.ID, 0)
	next, err = flow.pick()
	if err != nil || next.ID != ch.ID {
		t.Fatalf("pick after cooldown cleared = %+v, %v", next, err)
	}
	if flow.affinityPreferred != ch.ID {
		t.Fatal("the preference should be honored once the cooldown is cleared")
	}
}

// Hash mode picks deterministically per affinity key without shared state.
func TestRelayFlowAffinityHashMode(t *testing.T) {
	newTestDB(t)
	model.ResetAffinity()
	t.Cleanup(model.ResetAffinity)
	t.Cleanup(setting.ResetAffinityToDefaults)

	mustChannel(t, "hash-a", "gpt-4o", "sk-a", common.ChannelStatusEnabled)
	mustChannel(t, "hash-b", "gpt-4o", "sk-b", common.ChannelStatusEnabled)
	if err := setting.UpdateOption(setting.OptionKeyRoutingAffinityEnabled, "true"); err != nil {
		t.Fatalf("enable affinity failed: %v", err)
	}
	if err := setting.UpdateOption(setting.OptionKeyRoutingAffinityMode, setting.AffinityModeHash); err != nil {
		t.Fatalf("set hash mode failed: %v", err)
	}

	flow := newAffinityFlow(t, 21, common.DefaultGroup, "gpt-4o")
	first, err := flow.pick()
	if err != nil {
		t.Fatalf("pick failed: %v", err)
	}
	for i := 0; i < 30; i++ {
		got, err := flow.pick()
		if err != nil || got.ID != first.ID {
			t.Fatalf("hash pick %d = %d, %v; want %d", i, got.ID, err, first.ID)
		}
	}
	// Hash mode does not populate the sticky preference.
	if flow.affinityPreferred != 0 {
		t.Fatalf("hash mode should not set a sticky preference, got %d", flow.affinityPreferred)
	}
}

// Key-level affinity pins a specific upstream key and falls back when it is
// disabled.
func TestRelayFlowAffinityKeyLevel(t *testing.T) {
	newTestDB(t)
	model.ResetAffinity()
	t.Cleanup(model.ResetAffinity)
	t.Cleanup(setting.ResetAffinityToDefaults)

	ch := &model.Channel{
		Name:    "multi",
		Type:    common.DefaultChannelType,
		Key:     "sk-key-1\nsk-key-2",
		KeyMode: model.KeyModeRandom,
		Models:  "gpt-4o",
		Group:   common.DefaultGroup,
		Status:  common.ChannelStatusEnabled,
		Weight:  1,
	}
	if err := model.InsertChannel(ch); err != nil {
		t.Fatalf("InsertChannel failed: %v", err)
	}
	if err := setting.UpdateOption(setting.OptionKeyRoutingAffinityEnabled, "true"); err != nil {
		t.Fatalf("enable affinity failed: %v", err)
	}

	flow := newAffinityFlow(t, 55, common.DefaultGroup, "gpt-4o")
	model.RememberAffinity(flow.affinityKey(), ch.ID, 1, time.Minute)

	key, idx := flow.selectKey(ch)
	if idx != 1 || key != "sk-key-2" {
		t.Fatalf("selectKey = %q, %d; want sk-key-2, 1", key, idx)
	}

	// Disabling the remembered key must fall back to an enabled one.
	if _, err := model.DisableChannelKey(ch.ID, 1); err != nil {
		t.Fatalf("DisableChannelKey failed: %v", err)
	}
	reloaded, err := model.GetChannelByID(ch.ID)
	if err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	key, idx = flow.selectKey(reloaded)
	if idx == 1 {
		t.Fatalf("a disabled key must not be selected: %q, %d", key, idx)
	}
}

// The prefix scope is part of the affinity key only when prefix-cache affinity
// is enabled.
func TestRelayFlowAffinityPrefixScope(t *testing.T) {
	newTestDB(t)
	model.ResetAffinity()
	t.Cleanup(model.ResetAffinity)
	t.Cleanup(setting.ResetAffinityToDefaults)

	req := relayRequest{
		tokenID: 1, group: common.DefaultGroup, modelName: "gpt-4o", prefixHash: "deadbeef",
	}
	flow := newRelayFlow(context.Background(), httptest.NewRecorder(), req, dto.InboundOpenAI, "chat")
	t.Cleanup(flow.finish)

	if flow.affinityKey().Prefix != "" {
		t.Fatal("prefix must be ignored while prefix cache is disabled")
	}
	if err := setting.UpdateOption(setting.OptionKeyRoutingAffinityPrefixCache, "true"); err != nil {
		t.Fatalf("enable prefix cache failed: %v", err)
	}
	if got := flow.affinityKey().Prefix; got != "deadbeef" {
		t.Fatalf("prefix = %q, want deadbeef", got)
	}
}

package controller

import (
	"context"
	"net/http/httptest"
	"testing"

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

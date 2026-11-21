package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"starhub/common"
	"starhub/model"
	"starhub/relay/dto"
	"starhub/service"
	"starhub/setting"
)

// newRuleAffinityFlow builds a relayFlow carrying the request fields (body,
// header, path) that rule-driven affinity needs to evaluate a rule.
func newRuleAffinityFlow(t *testing.T, tokenID int64, group, modelName, body string) *relayFlow {
	t.Helper()
	req := relayRequest{
		tokenID:     tokenID,
		group:       group,
		modelName:   modelName,
		body:        []byte(body),
		header:      http.Header{},
		requestPath: "/v1/chat/completions",
	}
	flow := newRelayFlow(context.Background(), httptest.NewRecorder(), req, dto.InboundOpenAI, "chat")
	t.Cleanup(flow.finish)
	return flow
}

func enableRuleAffinityForRelay(t *testing.T, rulesJSON string) {
	t.Helper()
	if err := setting.UpdateOption(setting.OptionKeyRoutingAffinityEnabled, `true`); err != nil {
		t.Fatalf("enable affinity: %v", err)
	}
	if err := setting.UpdateOption(setting.OptionKeyRoutingAffinityRules, rulesJSON); err != nil {
		t.Fatalf("set rules: %v", err)
	}
}

// A matching rule with a gjson key source records the serving channel and
// the next request for the same affinity value lands on it.
func TestRelayFlowRuleAffinitySticky(t *testing.T) {
	newTestDB(t)
	t.Cleanup(model.ResetAffinity)
	t.Cleanup(service.ResetRuleAffinityCache)
	t.Cleanup(setting.ResetAffinityToDefaults)

	mustChannel(t, "rule-a", "gpt-4o", "sk-a", common.ChannelStatusEnabled)
	mustChannel(t, "rule-b", "gpt-4o", "sk-b", common.ChannelStatusEnabled)
	enableRuleAffinityForRelay(t, `[{"name":"session","model_regex":["^gpt-4o$"],"key_sources":[{"type":"gjson","path":"session_id"}],"include_rule_name":true}]`)

	// First request: no cache entry yet → rule matches but ChannelID=0.
	body := `{"model":"gpt-4o","session_id":"s-1","messages":[]}`
	flow := newRuleAffinityFlow(t, 7, common.DefaultGroup, "gpt-4o", body)
	flow.resolveRuleAffinity()
	if !flow.affinityRuleUsed || flow.affinityMeta.ChannelID != 0 {
		t.Fatalf("expected rule match with empty cache, used=%v channel=%d", flow.affinityRuleUsed, flow.affinityMeta.ChannelID)
	}
	ch, err := flow.pick()
	if err != nil {
		t.Fatalf("pick failed: %v", err)
	}
	// Success records the channel into the rule cache.
	flow.noteAffinity(ch, attemptSuccess)

	// Second request with the same session_id must prefer the recorded channel.
	flow2 := newRuleAffinityFlow(t, 7, common.DefaultGroup, "gpt-4o", body)
	ch2, err := flow2.pick()
	if err != nil {
		t.Fatalf("second pick failed: %v", err)
	}
	if ch2.ID != ch.ID {
		t.Fatalf("rule affinity should pin channel %d, got %d", ch.ID, ch2.ID)
	}
	// A different session_id should not inherit the pin.
	flow3 := newRuleAffinityFlow(t, 7, common.DefaultGroup, "gpt-4o", `{"model":"gpt-4o","session_id":"s-2","messages":[]}`)
	if _, err := flow3.pick(); err != nil {
		t.Fatalf("third pick failed: %v", err)
	}
	if flow3.affinityMeta.ChannelID != 0 {
		t.Fatalf("different session value should miss the cache, got channel %d", flow3.affinityMeta.ChannelID)
	}
}

// Strict session mode fails the request (503) when the bound channel is no
// longer selectable, instead of silently switching to another channel.
func TestRelayFlowRuleAffinityStrictFailsWhenBoundUnavailable(t *testing.T) {
	newTestDB(t)
	t.Cleanup(model.ResetAffinity)
	t.Cleanup(service.ResetRuleAffinityCache)
	t.Cleanup(setting.ResetAffinityToDefaults)

	a := mustChannel(t, "strict-a", "gpt-4o", "sk-a", common.ChannelStatusEnabled)
	mustChannel(t, "strict-b", "gpt-4o", "sk-b", common.ChannelStatusEnabled)
	enableRuleAffinityForRelay(t, `[{"name":"strict","model_regex":["^gpt-4o$"],"key_sources":[{"type":"gjson","path":"session_id"}],"session_mode":"strict","include_rule_name":true}]`)

	body := `{"model":"gpt-4o","session_id":"s-strict","messages":[]}`
	// Seed the cache by recording channel A as the bound channel.
	flow := newRuleAffinityFlow(t, 7, common.DefaultGroup, "gpt-4o", body)
	flow.resolveRuleAffinity()
	service.RecordRuleAffinity(flow.affinityMeta, a.ID)

	// Disable A. With strict mode, pick must refuse to fall back to B.
	if err := model.UpdateChannelStatus(a.ID, common.ChannelStatusManuallyDisabled); err != nil {
		t.Fatalf("disable channel A: %v", err)
	}
	flow2 := newRuleAffinityFlow(t, 7, common.DefaultGroup, "gpt-4o", body)
	_, err := flow2.pick()
	if !errors.Is(err, model.ErrStrictAffinity) {
		t.Fatalf("strict mode with unavailable bound must return ErrStrictAffinity, got %v", err)
	}
	if flow2.sw.status != http.StatusServiceUnavailable {
		t.Fatalf("strict failure status = %d, want 503", flow2.sw.status)
	}
}

// skip_retry_on_failure stops the retry loop after the bound channel fails,
// so no other channel is tried.
func TestRelayFlowRuleAffinitySkipRetry(t *testing.T) {
	newTestDB(t)
	t.Cleanup(model.ResetAffinity)
	t.Cleanup(service.ResetRuleAffinityCache)
	t.Cleanup(setting.ResetAffinityToDefaults)

	a := mustChannel(t, "skip-a", "gpt-4o", "sk-a", common.ChannelStatusEnabled)
	b := mustChannel(t, "skip-b", "gpt-4o", "sk-b", common.ChannelStatusEnabled)
	enableRuleAffinityForRelay(t, `[{"name":"skip","model_regex":["^gpt-4o$"],"key_sources":[{"type":"gjson","path":"session_id"}],"skip_retry_on_failure":true,"session_mode":"prefer","include_rule_name":true}]`)

	body := `{"model":"gpt-4o","session_id":"s-skip","messages":[]}`
	// Seed the cache so A is the bound channel.
	flow := newRuleAffinityFlow(t, 7, common.DefaultGroup, "gpt-4o", body)
	flow.resolveRuleAffinity()
	service.RecordRuleAffinity(flow.affinityMeta, a.ID)

	flow2 := newRuleAffinityFlow(t, 7, common.DefaultGroup, "gpt-4o", body)
	// runChannels with an attempt that always reports upstream error.
	flow2.runChannels(func(ch *model.Channel) attemptResult {
		return attemptResult{outcome: attemptUpstreamError, cause: errors.New("upstream 500")}
	})
	// The loop must stop after A fails; B must never be tried.
	if flow2.sent != 1 {
		t.Fatalf("skip_retry should stop after 1 send, got %d", flow2.sent)
	}
	if len(flow2.trail) != 1 || flow2.trail[0] != strconv.FormatInt(a.ID, 10) {
		t.Fatalf("only channel A should be tried, trail=%v", flow2.trail)
	}
	_ = b
}

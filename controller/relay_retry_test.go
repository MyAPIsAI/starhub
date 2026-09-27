package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"starhub/common"
	"starhub/model"
	"starhub/relay/dto"
	"starhub/setting"
)

// newRetryFlow builds a relayFlow the way a relay handler would, without
// opening a socket, so the failover loop can be exercised in isolation.
func newRetryFlow(t *testing.T, ctx context.Context, modelName string) (*relayFlow, *httptest.ResponseRecorder) {
	t.Helper()
	rr := httptest.NewRecorder()
	req := relayRequest{modelName: modelName, group: common.DefaultGroup}
	flow := newRelayFlow(ctx, rr, req, dto.InboundOpenAI, "chat")
	t.Cleanup(flow.finish)
	return flow, rr
}

// The failover loop must stop after RetryTimes extra sends (RetryTimes+1
// total), no matter how many candidate channels exist.
func TestRunChannelsRespectsRetryBudget(t *testing.T) {
	newTestDB(t)
	for i := 0; i < 8; i++ {
		mustChannel(t, fmt.Sprintf("rb-%d", i), "retry-model", "sk", common.ChannelStatusEnabled)
	}
	flow, rr := newRetryFlow(t, context.Background(), "retry-model")

	sends := 0
	flow.runChannels(func(ch *model.Channel) attemptResult {
		sends++
		return attemptResult{outcome: attemptUpstreamError, statusCode: http.StatusBadGateway,
			cause: errors.New("upstream down")}
	})

	if sends != common.RetryTimes+1 {
		t.Fatalf("upstream sends = %d, want %d (RetryTimes+1)", sends, common.RetryTimes+1)
	}
	if len(flow.tried) != common.RetryTimes+1 {
		t.Fatalf("tried channels = %d, want %d distinct", len(flow.tried), common.RetryTimes+1)
	}
	if !flow.budgetStop {
		t.Fatal("budgetStop should be set when the attempt budget is exhausted")
	}
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rr.Code)
	}
}

// Unsupported skips (a channel that does not serve the mode) must not
// consume the send budget: the loop keeps looking past them.
func TestRunChannelsUnsupportedSkipsDoNotConsumeBudget(t *testing.T) {
	newTestDB(t)
	for i := 0; i < 8; i++ {
		mustChannel(t, fmt.Sprintf("us-%d", i), "skip-model", "sk", common.ChannelStatusEnabled)
	}
	flow, _ := newRetryFlow(t, context.Background(), "skip-model")

	skipped, calls := 0, 0
	flow.runChannels(func(ch *model.Channel) attemptResult {
		calls++
		if skipped < 5 {
			skipped++
			return attemptResult{outcome: attemptUnsupported, cause: errors.New("mode unsupported")}
		}
		return attemptResult{outcome: attemptSuccess, statusCode: http.StatusOK}
	})

	if skipped != 5 || calls != 6 {
		t.Fatalf("skipped=%d calls=%d, want 5 skips then a success", skipped, calls)
	}
	if flow.sent != 0 {
		t.Fatalf("sent = %d, want 0 (unsupported skips must not consume the send budget)", flow.sent)
	}
}

// Once any part of the client response has been committed the loop must not
// start another attempt: a second upstream stream would be appended to the
// first one's bytes.
func TestRunChannelsStopsAfterResponseCommitted(t *testing.T) {
	newTestDB(t)
	for i := 0; i < 5; i++ {
		mustChannel(t, fmt.Sprintf("rc-%d", i), "committed-model", "sk", common.ChannelStatusEnabled)
	}
	flow, rr := newRetryFlow(t, context.Background(), "committed-model")

	sends := 0
	flow.runChannels(func(ch *model.Channel) attemptResult {
		sends++
		flow.w.WriteHeader(http.StatusOK)
		_, _ = flow.w.Write([]byte("partial"))
		return attemptResult{outcome: attemptUpstreamError, cause: errors.New("stream broke")}
	})

	if sends != 1 {
		t.Fatalf("sends = %d, want 1 (no retry after the response was committed)", sends)
	}
	if rr.Code != http.StatusOK || rr.Body.String() != "partial" {
		t.Fatalf("committed response = %d %q, want 200 \"partial\"", rr.Code, rr.Body.String())
	}
}

// A cancelled client request must not trigger another upstream send.
func TestRunChannelsStopsOnClientCancel(t *testing.T) {
	newTestDB(t)
	for i := 0; i < 5; i++ {
		mustChannel(t, fmt.Sprintf("cx-%d", i), "cancel-model", "sk", common.ChannelStatusEnabled)
	}
	ctx, cancel := context.WithCancel(context.Background())
	flow, _ := newRetryFlow(t, ctx, "cancel-model")

	sends := 0
	flow.runChannels(func(ch *model.Channel) attemptResult {
		sends++
		cancel()
		return attemptResult{outcome: attemptUpstreamError, cause: errors.New("first failed")}
	})

	if sends != 1 {
		t.Fatalf("sends = %d, want 1 (client cancel stops new attempts)", sends)
	}
}

// An uncertain side-effecting attempt is terminal: it must not be retried
// across channels and the flow records the uncertainty.
func TestRunChannelsUncertainStopsWithoutRetry(t *testing.T) {
	newTestDB(t)
	for i := 0; i < 5; i++ {
		mustChannel(t, fmt.Sprintf("uc-%d", i), "uncertain-model", "sk", common.ChannelStatusEnabled)
	}
	flow, rr := newRetryFlow(t, context.Background(), "uncertain-model")

	sends := 0
	flow.runChannels(func(ch *model.Channel) attemptResult {
		sends++
		return attemptResult{outcome: attemptUncertain, cause: errors.New("result unknown")}
	})

	if sends != 1 {
		t.Fatalf("sends = %d, want 1 (an uncertain result must not be retried)", sends)
	}
	if !flow.uncertain {
		t.Fatal("flow.uncertain should be set")
	}
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 for an uncertain result", rr.Code)
	}
}

// An uncertain side-effect failure must preserve the affinity preference:
// the channel may not be broken, so the next request still prefers it.
func TestNoteAffinityUncertainPreservesPreference(t *testing.T) {
	newTestDB(t)
	model.ResetAffinity()
	t.Cleanup(model.ResetAffinity)
	t.Cleanup(setting.ResetAffinityToDefaults)

	ch := mustChannel(t, "aff-uncertain", "gpt-4o", "sk", common.ChannelStatusEnabled)
	if err := setting.UpdateOption(setting.OptionKeyRoutingAffinityEnabled, "true"); err != nil {
		t.Fatalf("enable affinity failed: %v", err)
	}
	flow := newAffinityFlow(t, 9, common.DefaultGroup, "gpt-4o")
	flow.noteAffinity(ch, attemptSuccess)
	flow.noteAffinity(ch, attemptUncertain)
	if got, ok := model.AffinityChannel(flow.affinityKey()); !ok || got != ch.ID {
		t.Fatalf("uncertain must not clear the preference: %d, %v", got, ok)
	}
}

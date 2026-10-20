package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"starhub/common"
	"starhub/model"
	"starhub/relay"
)

// TestBILL004FailoverThenSuccess verifies that when the first channel
// fails with a retryable 5xx and the second channel succeeds, the user
// is charged exactly once for the final successful response — no double
// refund, no double charge, no leftover pre-consume. This is BILL-004.
func TestBILL004FailoverThenSuccess(t *testing.T) {
	// First upstream: always returns 500 (retryable).
	failingUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error":{"message":"upstream down","type":"server_error","code":"internal"}}`)
	}))
	t.Cleanup(failingUpstream.Close)

	// Second upstream: returns success with known usage (100 prompt, 50 completion).
	healthyUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"id":"mock","object":"chat.completion","model":"gpt-bill-004","choices":[{"index":0,"message":{"role":"assistant","content":"hello world"}}],"usage":{"prompt_tokens":100,"completion_tokens":50}}`)
	}))
	t.Cleanup(healthyUpstream.Close)

	env := setupRelayApp(t)

	// 1. Set user quota to a known value.
	userBefore, _ := model.GetUserByID(env.rootID)
	res := doRequest(env.base, "PUT", "/api/user/", env.session, map[string]any{
		"id":             env.rootID,
		"quota":          10000000,
		"expected_quota": userBefore.Quota,
	})
	if res.Status != 200 {
		t.Fatalf("set user quota failed: %d %s", res.Status, res.Body)
	}
	userBefore, _ = model.GetUserByID(env.rootID)

	// 2. Create two channels: failing (higher priority) and healthy (lower).
	chFail := createChannelID(t, env, map[string]any{
		"name":     "always-500",
		"key":      upstreamKey,
		"base_url": failingUpstream.URL,
		"models":   "gpt-bill-004",
		"priority": 30,
	})
	chOK := createChannelID(t, env, map[string]any{
		"name":     "healthy",
		"key":      upstreamKey,
		"base_url": healthyUpstream.URL,
		"models":   "gpt-bill-004",
		"priority": 10,
	})

	tokenBefore, _ := model.GetTokenByID(env.tokenID)

	// 3. Make a relay request — first channel 500s, failover to second succeeds.
	chatBody := map[string]any{
		"model":    "gpt-bill-004",
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	}
	res = doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, chatBody)
	if res.Status != 200 {
		t.Fatalf("relay should succeed via failover, got %d: %s", res.Status, res.Body)
	}

	// 4. Compute the expected charge: 100 prompt + 50 completion tokens.
	expected := relay.ComputeQuota("gpt-bill-004", "default", 100, 50)
	if expected <= 0 {
		t.Fatalf("expected quota should be positive, got %d", expected)
	}

	// 5. Verify user quota decreased by exactly expected — one charge, no double.
	userAfter, _ := model.GetUserByID(env.rootID)
	expectedUserQuota := userBefore.Quota - expected
	if userAfter.Quota != expectedUserQuota {
		t.Fatalf("user quota = %d, want %d (before=%d, expected charge=%d, no double charge)",
			userAfter.Quota, expectedUserQuota, userBefore.Quota, expected)
	}

	// 6. Verify user.UsedQuota increased by exactly expected.
	if userAfter.UsedQuota != userBefore.UsedQuota+expected {
		t.Fatalf("user used quota = %d, want %d (before=%d + one charge %d)",
			userAfter.UsedQuota, userBefore.UsedQuota+expected, userBefore.UsedQuota, expected)
	}

	// 7. Verify token quota decreased by exactly expected.
	tokenAfter, _ := model.GetTokenByID(env.tokenID)
	expectedTokenQuota := tokenBefore.RemainQuota - expected
	if tokenAfter.RemainQuota != expectedTokenQuota {
		t.Fatalf("token remain quota = %d, want %d (before=%d, one charge=%d)",
			tokenAfter.RemainQuota, expectedTokenQuota, tokenBefore.RemainQuota, expected)
	}

	// 8. Verify exactly one successful log entry with correct usage and channel trace.
	logs, total, err := model.ListLogs(env.rootID, 1, 10, "gpt-bill-004")
	if err != nil || total != 1 || len(logs) != 1 {
		t.Fatalf("expected 1 log, got total=%d len=%d err=%v", total, len(logs), err)
	}
	if logs[0].Status != 1 {
		t.Fatalf("log status = %d, want 1 (success)", logs[0].Status)
	}
	if logs[0].Quota != expected {
		t.Fatalf("log quota = %d, want %d", logs[0].Quota, expected)
	}
	if logs[0].PromptTokens != 100 || logs[0].CompletionTokens != 50 {
		t.Fatalf("log tokens wrong: prompt=%d completion=%d", logs[0].PromptTokens, logs[0].CompletionTokens)
	}
	// Channel trace should show failover: chFail -> chOK
	wantTrace := fmt.Sprintf("%d->%d", chFail, chOK)
	if logs[0].UsedChannels != wantTrace {
		t.Fatalf("channel trace = %q, want %q", logs[0].UsedChannels, wantTrace)
	}

	// 9. The relay quota system charges user/token/channel directly; the
	//     wallet/ledger system is separate (recharge-only). Verify no
	//     unexpected call_summary ledger entries were created.
	entries, err := model.ListLedger(env.rootID)
	if err != nil {
		t.Fatalf("ListLedger failed: %v", err)
	}
	for _, e := range entries {
		if e.Type == "call_summary" {
			t.Fatalf("unexpected call_summary ledger entry for trace %s — relay charges via quota not wallet", e.TraceID)
		}
	}

	// 10. Verify both channels remain enabled (transient 5xx doesn't auto-disable).
	chFailAfter, _ := model.GetChannelByID(chFail)
	if chFailAfter.Status != common.ChannelStatusEnabled {
		t.Fatalf("failing channel status = %d, want %d (enabled)", chFailAfter.Status, common.ChannelStatusEnabled)
	}
	chOKAfter, _ := model.GetChannelByID(chOK)
	if chOKAfter.Status != common.ChannelStatusEnabled {
		t.Fatalf("healthy channel status = %d, want %d (enabled)", chOKAfter.Status, common.ChannelStatusEnabled)
	}

	// 11. Verify only the successful channel has used quota (not the failing one).
	if chFailAfter.UsedQuota != 0 {
		t.Fatalf("failing channel used quota = %d, want 0 (no charge on failed attempt)", chFailAfter.UsedQuota)
	}
	if chOKAfter.UsedQuota != expected {
		t.Fatalf("healthy channel used quota = %d, want %d", chOKAfter.UsedQuota, expected)
	}

	t.Logf("BILL-004: failover %d→%d, charged %d once, user %d→%d, token %d→%d, 1 log, 1 ledger, both channels enabled",
		chFail, chOK, expected,
		userBefore.Quota, userAfter.Quota,
		tokenBefore.RemainQuota, tokenAfter.RemainQuota)
}

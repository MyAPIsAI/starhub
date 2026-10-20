package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"starhub/common"
	"starhub/model"
)

// TestBILL002FullFailureRefund verifies that when all upstream channels
// fail with retryable errors, the pre-consumed quota is fully refunded:
// user quota, token quota and channel usage all return to their pre-request
// state. This is BILL-002 from the P21 test strategy.
func TestBILL002FullFailureRefund(t *testing.T) {
	// Mock upstream that always returns 500 (retryable).
	failingUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error":{"message":"upstream down","type":"server_error","code":"internal"}}`)
	}))
	t.Cleanup(failingUpstream.Close)

	env := setupRelayApp(t)

	// 1. Create a channel pointing at the always-failing upstream.
	createChannelID(t, env, map[string]any{
		"name":     "always-500",
		"key":      upstreamKey,
		"base_url": failingUpstream.URL,
		"models":   "gpt-bill-test",
		"priority": 10,
	})

	// 2. Set user quota to a known value (1000 quota = $0.002).
	//    Use the admin update endpoint with optimistic lock.
	userBefore, _ := model.GetUserByID(env.rootID)
	res := doRequest(env.base, "PUT", "/api/user/", env.session, map[string]any{
		"id":              env.rootID,
		"quota":           1000,
		"expected_quota":  userBefore.Quota,
	})
	if res.Status != 200 {
		t.Fatalf("set user quota failed: %d %s", res.Status, res.Body)
	}

	// 3. Record pre-request state for user, token and channel.
	userBefore, _ = model.GetUserByID(env.rootID)
	if userBefore.Quota != 1000 {
		t.Fatalf("user quota should be 1000, got %d", userBefore.Quota)
	}
	tokenBefore, _ := model.GetTokenByID(env.tokenID)
	channelsBefore, _ := model.ListChannels()
	userUsedBefore := userBefore.UsedQuota
	tokenRemainBefore := tokenBefore.RemainQuota

	// 4. Make a relay request — it will pre-consume, try the channel,
	//    get 500, retry (same channel since only one), exhaust budget,
	//    and ultimately fail. Pre-consumed quota must be refunded.
	chatBody := map[string]any{
		"model":    "gpt-bill-test",
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	}
	res = doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, chatBody)
	if res.Status == 200 {
		t.Fatalf("expected relay to fail, got 200: %s", res.Body)
	}

	// 5. Verify user quota is fully restored — no charge for failure.
	userAfter, _ := model.GetUserByID(env.rootID)
	if userAfter.Quota != userBefore.Quota {
		t.Fatalf("user quota after full failure = %d, want %d (should be fully refunded)",
			userAfter.Quota, userBefore.Quota)
	}

	// 6. Verify user.UsedQuota did not increase — no successful consumption.
	if userAfter.UsedQuota != userUsedBefore {
		t.Fatalf("user used quota after failure = %d, want %d (no consumption on failure)",
			userAfter.UsedQuota, userUsedBefore)
	}

	// 7. Verify token quota is fully restored.
	tokenAfter, _ := model.GetTokenByID(env.tokenID)
	if tokenAfter.RemainQuota != tokenRemainBefore {
		t.Fatalf("token remain quota after failure = %d, want %d (should be fully refunded)",
			tokenAfter.RemainQuota, tokenRemainBefore)
	}

	// 8. Verify channel used quota did not increase.
	channelsAfter, _ := model.ListChannels()
	for _, ch := range channelsAfter {
		for _, chb := range channelsBefore {
			if ch.ID == chb.ID {
				if ch.UsedQuota != chb.UsedQuota {
					t.Fatalf("channel %s used quota after failure = %d, want %d",
						ch.Name, ch.UsedQuota, chb.UsedQuota)
				}
			}
		}
	}

	// 9. Verify NO log entry is created for the failed request
	//    (the relay intentionally does not write usage logs for failures
	//    that never reached a successful upstream response).
	logs, total, err := model.ListLogs(env.rootID, 1, 10, "gpt-bill-test")
	if err != nil {
		t.Fatalf("ListLogs failed: %v", err)
	}
	if total != 0 || len(logs) != 0 {
		t.Fatalf("failed relay must not write usage logs, got total=%d len=%d", total, len(logs))
	}

	// 10. Verify no wallet ledger entry was created for this failed call.
	entries, err := model.ListLedger(env.rootID)
	if err != nil {
		t.Fatalf("ListLedger failed: %v", err)
	}
	for _, e := range entries {
		if e.Type == "call_summary" {
			t.Fatalf("found call_summary ledger entry — should not charge for failed call")
		}
	}

	// 11. Verify channel status remains enabled (transient 5xx doesn't auto-disable).
	for _, ch := range channelsAfter {
		if ch.Name == "always-500" && ch.Status != common.ChannelStatusEnabled {
			t.Fatalf("channel status = %d, want %d (enabled) — transient 5xx must not auto-disable",
				ch.Status, common.ChannelStatusEnabled)
		}
	}

	t.Logf("BILL-002: full failure refund verified — user quota %d→%d, token %d→%d, 0 logs, no ledger charge",
		userBefore.Quota, userAfter.Quota, tokenRemainBefore, tokenAfter.RemainQuota)
}

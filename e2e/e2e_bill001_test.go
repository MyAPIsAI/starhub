package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"starhub/model"
	"starhub/relay"
)

// TestBILL001NormalSettlement verifies the normal billing flow: pre-consume
// → successful upstream response → settlement against actual usage →
// correct final balances for user, token, channel and log.
// This is BILL-001 from the P21 test strategy.
//
// The mock upstream returns usage of 100 prompt + 50 completion tokens.
// The expected charge is computed by relay.ComputeQuota and verified
// independently in the test — not by calling the same function twice.
func TestBILL001NormalSettlement(t *testing.T) {
	// Mock upstream that always succeeds with known usage.
	healthyUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+upstreamKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"id":"mock","object":"chat.completion","model":"gpt-bill-001","choices":[{"index":0,"message":{"role":"assistant","content":"hello world"}}],"usage":{"prompt_tokens":100,"completion_tokens":50}}`)
	}))
	t.Cleanup(healthyUpstream.Close)

	env := setupRelayApp(t)

	// 1. Set user quota to a known value (10,000,000 = $20).
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
	if userBefore.Quota != 10000000 {
		t.Fatalf("user quota should be 10000000, got %d", userBefore.Quota)
	}

	// 2. Create a channel pointing at the healthy upstream.
	createChannelID(t, env, map[string]any{
		"name":     "healthy",
		"key":      upstreamKey,
		"base_url": healthyUpstream.URL,
		"models":   "gpt-bill-001",
		"priority": 10,
	})

	// 3. Record pre-request state.
	tokenBefore, _ := model.GetTokenByID(env.tokenID)
	channelsBefore, _ := model.ListChannels()
	userUsedBefore := userBefore.UsedQuota
	tokenRemainBefore := tokenBefore.RemainQuota

	// 4. Make a non-stream relay request with known usage.
	chatBody := map[string]any{
		"model":    "gpt-bill-001",
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	}
	res = doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, chatBody)
	if res.Status != 200 {
		t.Fatalf("relay should succeed, got %d: %s", res.Status, res.Body)
	}

	// 5. Compute expected charge independently from relay.ComputeQuota.
	expected := relay.ComputeQuota("gpt-bill-001", "default", 100, 50)
	if expected <= 0 {
		t.Fatalf("expected quota should be positive, got %d", expected)
	}

	// 6. Verify user quota decreased by exactly expected (one charge).
	userAfter, _ := model.GetUserByID(env.rootID)
	expectedUserQuota := userBefore.Quota - expected
	if userAfter.Quota != expectedUserQuota {
		t.Fatalf("user quota = %d, want %d (before=%d, charge=%d)",
			userAfter.Quota, expectedUserQuota, userBefore.Quota, expected)
	}

	// 7. Verify user.UsedQuota increased by exactly expected.
	if userAfter.UsedQuota != userUsedBefore+expected {
		t.Fatalf("user used quota = %d, want %d (before=%d + %d)",
			userAfter.UsedQuota, userUsedBefore+expected, userUsedBefore, expected)
	}

	// 8. Verify token RemainQuota decreased by exactly expected.
	tokenAfter, _ := model.GetTokenByID(env.tokenID)
	expectedTokenQuota := tokenRemainBefore - expected
	if tokenAfter.RemainQuota != expectedTokenQuota {
		t.Fatalf("token remain quota = %d, want %d (before=%d, charge=%d)",
			tokenAfter.RemainQuota, expectedTokenQuota, tokenRemainBefore, expected)
	}

	// 9. Verify exactly one successful log entry with correct fields.
	logs, total, err := model.ListLogs(env.rootID, 1, 10, "gpt-bill-001")
	if err != nil || total != 1 || len(logs) != 1 {
		t.Fatalf("expected 1 log, got total=%d len=%d err=%v", total, len(logs), err)
	}
	if logs[0].Status != 1 {
		t.Fatalf("log status = %d, want 1 (success)", logs[0].Status)
	}
	if logs[0].Quota != expected {
		t.Fatalf("log quota = %d, want %d (实收)", logs[0].Quota, expected)
	}
	if logs[0].UnfundedQuota != 0 {
		t.Fatalf("log unfunded quota = %d, want 0 (欠收=0, user had enough)", logs[0].UnfundedQuota)
	}
	if logs[0].PromptTokens != 100 || logs[0].CompletionTokens != 50 {
		t.Fatalf("log tokens: prompt=%d want 100, completion=%d want 50",
			logs[0].PromptTokens, logs[0].CompletionTokens)
	}

	// 10. Verify channel UsedQuota increased by exactly expected.
	channelsAfter, _ := model.ListChannels()
	for _, ch := range channelsAfter {
		for _, chb := range channelsBefore {
			if ch.ID == chb.ID && ch.Name == "healthy" {
				if ch.UsedQuota != chb.UsedQuota+expected {
					t.Fatalf("channel used quota = %d, want %d (before=%d + %d)",
						ch.UsedQuota, chb.UsedQuota+expected, chb.UsedQuota, expected)
				}
			}
		}
	}

	// 11. Amount conservation: 期初 - 期末 = 实收.
	// user: 期初 quota - 期末 quota = expected (实收)
	if userBefore.Quota-userAfter.Quota != expected {
		t.Fatalf("amount conservation failed: user delta=%d, expected=%d",
			userBefore.Quota-userAfter.Quota, expected)
	}
	// token: 期初 remain - 期末 remain = expected (实收)
	if tokenBefore.RemainQuota-tokenAfter.RemainQuota != expected {
		t.Fatalf("amount conservation failed: token delta=%d, expected=%d",
			tokenBefore.RemainQuota-tokenAfter.RemainQuota, expected)
	}

	// 12. Verify no wallet/ledger charge (relay uses quota, not wallet).
	entries, err := model.ListLedger(env.rootID)
	if err != nil {
		t.Fatalf("ListLedger failed: %v", err)
	}
	for _, e := range entries {
		if e.Type == "call_summary" {
			t.Fatalf("unexpected call_summary ledger entry — relay charges via quota not wallet")
		}
	}

	t.Logf("BILL-001: charged %d once, user %d→%d, token %d→%d, 1 log (实收=%d 欠收=0), channel +used",
		expected,
		userBefore.Quota, userAfter.Quota,
		tokenBefore.RemainQuota, tokenAfter.RemainQuota,
		expected)
}

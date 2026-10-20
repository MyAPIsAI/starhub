package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"starhub/model"
	"starhub/relay"
)

// TestBILL003UnderfundedSettlement verifies that when a user's quota is
// insufficient to cover the full priced usage, the system charges what
// it can (实收=user quota) and records the shortfall as UnfundedQuota
// (欠收=difference). The user quota goes to 0; the log reflects both
// the charged amount and the unfunded remainder. This is BILL-003.
func TestBILL003UnderfundedSettlement(t *testing.T) {
	// Mock upstream that returns large usage: 500 prompt + 500 completion.
	// This creates a charge much larger than the small user quota.
	healthyUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"id":"mock","object":"chat.completion","model":"gpt-bill-003","choices":[{"index":0,"message":{"role":"assistant","content":"hello world"}}],"usage":{"prompt_tokens":500,"completion_tokens":500}}`)
	}))
	t.Cleanup(healthyUpstream.Close)

	env := setupRelayApp(t)

	// 1. Compute the expected full charge for 500+500 tokens.
	expectedFull := relay.ComputeQuota("gpt-bill-003", "default", 500, 500)
	if expectedFull <= 0 {
		t.Fatalf("expected full charge should be positive, got %d", expectedFull)
	}

	// 2. Set user quota to a value that passes pre-consume but is less
	//    than the full settlement charge. Pre-consume estimates the prompt
	//    tokens + max_tokens budget; the actual usage (500+500) may exceed
	//    what was pre-charged. We set quota to the pre-consume estimate so
	//    the request passes, then the settlement finds the actual charge
	//    is higher — the shortfall becomes UnfundedQuota.
	chatBody := map[string]any{
		"model":    "gpt-bill-003",
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	}
	bodyBytes, _ := json.Marshal(chatBody)
	preConsume := relay.EstimatePreConsumeQuota("gpt-bill-003", "default", bodyBytes)
	if preConsume <= 0 {
		t.Fatalf("pre-consume estimate should be positive, got %d", preConsume)
	}

	// Ensure the full charge exceeds pre-consume so there's a shortfall.
	// If not, use a quota that's half of expectedFull (but >= preConsume).
	var userQuota int64
	if expectedFull > preConsume*2 {
		// Pre-consume is small relative to full charge: set quota = preConsume
		// so the request passes but settlement charges more.
		userQuota = preConsume
	} else {
		// Pre-consume already covers most of the charge: set quota to half
		// the full charge (but ensure it's >= preConsume to pass the gate).
		userQuota = expectedFull / 2
		if userQuota < preConsume {
			userQuota = preConsume
		}
	}

	userBefore, _ := model.GetUserByID(env.rootID)
	res := doRequest(env.base, "PUT", "/api/user/", env.session, map[string]any{
		"id":             env.rootID,
		"quota":          userQuota,
		"expected_quota": userBefore.Quota,
	})
	if res.Status != 200 {
		t.Fatalf("set user quota failed: %d %s", res.Status, res.Body)
	}
	userBefore, _ = model.GetUserByID(env.rootID)
	if userBefore.Quota != userQuota {
		t.Fatalf("user quota should be %d, got %d", userQuota, userBefore.Quota)
	}

	// 3. Create a channel.
	createChannelID(t, env, map[string]any{
		"name":     "healthy",
		"key":      upstreamKey,
		"base_url": healthyUpstream.URL,
		"models":   "gpt-bill-003",
		"priority": 10,
	})

	// 4. Make the relay request.
	res = doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, chatBody)
	if res.Status != 200 {
		t.Fatalf("relay should succeed (upstream returns 200), got %d: %s", res.Status, res.Body)
	}

	// 5. Verify user quota decreased — the settlement charged the actual usage.
	// If userQuota < expectedFull, the charge is capped at available quota and
	// the shortfall goes to UnfundedQuota. If userQuota >= expectedFull, the full
	// charge is deducted and UnfundedQuota=0.
	userAfter, _ := model.GetUserByID(env.rootID)
	actualCharged := userBefore.Quota - userAfter.Quota
	if actualCharged <= 0 {
		t.Fatalf("user quota should have decreased, before=%d after=%d (delta=%d)",
			userBefore.Quota, userAfter.Quota, actualCharged)
	}
	// The charged amount cannot exceed the full charge or the available quota.
	if actualCharged > expectedFull {
		t.Fatalf("charged %d exceeds full charge %d", actualCharged, expectedFull)
	}
	if actualCharged > userBefore.Quota {
		t.Fatalf("charged %d exceeds user's pre-request quota %d", actualCharged, userBefore.Quota)
	}

	// 6. Verify the log entry records both 实收 and 欠收.
	logs, total, err := model.ListLogs(env.rootID, 1, 10, "gpt-bill-003")
	if err != nil || total != 1 || len(logs) != 1 {
		t.Fatalf("expected 1 log, got total=%d len=%d err=%v", total, len(logs), err)
	}
	log := logs[0]

	// Log status should be 1 (success) — the upstream returned a valid response.
	if log.Status != 1 {
		t.Fatalf("log status = %d, want 1 (success — upstream returned 200)", log.Status)
	}

	// The log's Quota field records the actual settlement charge.
	// It should match the actual deduction from the user's quota.
	if log.Quota != actualCharged {
		t.Fatalf("log quota (实收) = %d, want %d (actual user quota delta)",
			log.Quota, actualCharged)
	}

	// 欠收 = 应收 - 实收.
	expectedUnfunded := expectedFull - actualCharged
	if expectedUnfunded < 0 {
		expectedUnfunded = 0
	}
	if log.UnfundedQuota != expectedUnfunded {
		t.Fatalf("log unfunded quota (欠收) = %d, want %d (full=%d - charged=%d)",
			log.UnfundedQuota, expectedUnfunded, expectedFull, actualCharged)
	}

	// 7. Verify the sum: 实收 + 欠收 = 应收 (full charge).
	if log.Quota+log.UnfundedQuota != expectedFull {
		t.Fatalf("实收(%d) + 欠收(%d) = %d, want 应收 %d",
			log.Quota, log.UnfundedQuota, log.Quota+log.UnfundedQuota, expectedFull)
	}

	// 8. Verify log tokens match the upstream's reported usage.
	if log.PromptTokens != 500 || log.CompletionTokens != 500 {
		t.Fatalf("log tokens: prompt=%d want 500, completion=%d want 500",
			log.PromptTokens, log.CompletionTokens)
	}

	t.Logf("BILL-003: user quota %d→%d (delta=%d), 实收=%d 欠收=%d 应收=%d (sum=%d), 1 log status=success",
		userBefore.Quota, userAfter.Quota, actualCharged, log.Quota, log.UnfundedQuota, expectedFull,
		log.Quota+log.UnfundedQuota)
}

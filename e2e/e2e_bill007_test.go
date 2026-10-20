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

// TestBILL007RechargeThenConsume verifies the cross-account bridge:
// user starts with zero relay quota → admin sets quota via recharge
// (PUT /api/user quota) → user makes a relay call → consumption is
// charged correctly. This is BILL-007 from the P21 test strategy.
//
// The test does NOT seed quota directly via DB; it uses the real admin
// API to set quota, then verifies the relay can consume it.
func TestBILL007RechargeThenConsume(t *testing.T) {
	// Mock upstream with known usage (100 prompt + 50 completion).
	healthyUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"id":"mock","object":"chat.completion","model":"gpt-bill-007","choices":[{"index":0,"message":{"role":"assistant","content":"hello world"}}],"usage":{"prompt_tokens":100,"completion_tokens":50}}`)
	}))
	t.Cleanup(healthyUpstream.Close)

	env := setupRelayApp(t)

	// 1. Set user quota to 0 — no relay budget.
	userBefore, _ := model.GetUserByID(env.rootID)
	res := doRequest(env.base, "PUT", "/api/user/", env.session, map[string]any{
		"id":             env.rootID,
		"quota":          0,
		"expected_quota": userBefore.Quota,
	})
	if res.Status != 200 {
		t.Fatalf("set user quota to 0 failed: %d %s", res.Status, res.Body)
	}
	userZero, _ := model.GetUserByID(env.rootID)
	if userZero.Quota != 0 {
		t.Fatalf("user quota should be 0, got %d", userZero.Quota)
	}

	// 2. Verify relay call with 0 quota is rejected (insufficient_quota).
	chatBody := map[string]any{
		"model":    "gpt-bill-007",
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	}
	createChannelID(t, env, map[string]any{
		"name":     "healthy",
		"key":      upstreamKey,
		"base_url": healthyUpstream.URL,
		"models":   "gpt-bill-007",
		"priority": 10,
	})
	res = doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, chatBody)
	if res.Status != 403 {
		t.Fatalf("relay with 0 quota should be rejected (403), got %d: %s", res.Status, res.Body)
	}

	// 3. Admin "recharges" the user via PUT /api/user quota (admin path).
	//    This is the admin top-up flow that credits relay quota.
	//    Use a value that covers the pre-consume estimate and the actual charge.
	expectedCharge := relay.ComputeQuota("gpt-bill-007", "default", 100, 50)
	if expectedCharge <= 0 {
		t.Fatalf("expected charge should be positive, got %d", expectedCharge)
	}

	// Compute pre-consume estimate to make sure we set enough quota.
	bodyBytes, _ := json.Marshal(chatBody)
	preConsume := relay.EstimatePreConsumeQuota("gpt-bill-007", "default", bodyBytes)
	t.Logf("BILL-007: expectedCharge=%d, preConsume=%d", expectedCharge, preConsume)

	// Set quota to max(preConsume, expectedCharge) * 10 to ensure it passes.
	rechargeAmount := preConsume * 10
	if rechargeAmount < expectedCharge*10 {
		rechargeAmount = expectedCharge * 10
	}
	res = doRequest(env.base, "PUT", "/api/user/", env.session, map[string]any{
		"id":             env.rootID,
		"quota":          rechargeAmount,
		"expected_quota": userZero.Quota, // 0
	})
	if res.Status != 200 {
		t.Fatalf("admin top-up failed: %d %s", res.Status, res.Body)
	}

	// 4. Verify quota was credited — NOT via seed/fixture, via the real API.
	userFunded, _ := model.GetUserByID(env.rootID)
	if userFunded.Quota != rechargeAmount {
		t.Fatalf("user quota after top-up = %d, want %d (API credited, not seeded)",
			userFunded.Quota, rechargeAmount)
	}
	if userFunded.Quota == 0 {
		t.Fatal("quota is still 0 after admin top-up — recharge bridge is broken")
	}

	// 4b. The token from setupRelayApp has default quota which may be
	//     smaller than the pre-consume estimate. Update it to unlimited
	//     so the token quota check doesn't block the relay.
	t.Logf("BILL-007: userFunded.Quota=%d, tokenID=%d", userFunded.Quota, env.tokenID)
	res = doRequest(env.base, "PUT", "/api/token/", env.session, map[string]any{
		"id":               env.tokenID,
		"unlimited_quota":  true,
	})
	if res.Status != 200 {
		t.Fatalf("set token to unlimited failed: %d %s", res.Status, res.Body)
	}
	tokenUpdated, _ := model.GetTokenByID(env.tokenID)
	t.Logf("BILL-007: token unlimited=%v remain=%d", tokenUpdated.UnlimitedQuota, tokenUpdated.RemainQuota)

	// 5. Now make the relay call — it should succeed and consume quota.
	res = doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, chatBody)
	if res.Status != 200 {
		t.Fatalf("relay after top-up should succeed, got %d: %s", res.Status, res.Body)
	}

	// 6. Verify quota decreased by exactly the expected charge.
	userAfter, _ := model.GetUserByID(env.rootID)
	expectedAfter := rechargeAmount - expectedCharge
	if userAfter.Quota != expectedAfter {
		t.Fatalf("user quota after relay = %d, want %d (top-up=%d - charge=%d)",
			userAfter.Quota, expectedAfter, rechargeAmount, expectedCharge)
	}

	// 7. Verify user.UsedQuota increased by exactly the expected charge.
	if userAfter.UsedQuota != expectedCharge {
		t.Fatalf("user used quota = %d, want %d", userAfter.UsedQuota, expectedCharge)
	}

	// 8. Verify token is unlimited (remain_quota was zeroed on switch).
	tokenAfter, _ := model.GetTokenByID(env.tokenID)
	if !tokenAfter.UnlimitedQuota {
		t.Fatalf("token should be unlimited after top-up flow")
	}

	// 9. Verify exactly one successful log entry.
	logs, total, err := model.ListLogs(env.rootID, 1, 10, "gpt-bill-007")
	if err != nil || total != 1 || len(logs) != 1 {
		t.Fatalf("expected 1 success log, got total=%d len=%d err=%v", total, len(logs), err)
	}
	if logs[0].Status != 1 {
		t.Fatalf("log status = %d, want 1 (success)", logs[0].Status)
	}
	if logs[0].Quota != expectedCharge {
		t.Fatalf("log quota = %d, want %d", logs[0].Quota, expectedCharge)
	}
	if logs[0].UnfundedQuota != 0 {
		t.Fatalf("log unfunded quota = %d, want 0 (user had enough)", logs[0].UnfundedQuota)
	}

	t.Logf("BILL-007: user quota 0 → rejected (403) → admin top-up %d → relay success → consumed %d → final %d",
		rechargeAmount, expectedCharge, userAfter.Quota)
}

package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"starhub/common"
	"starhub/model"
)

// TestRetry6xxNonRetryable4xx verifies section 6.2 of the P21 test strategy:
// "参数/权限类不可重试错误 → 精确错误映射；不访问下一渠道；按规则释放预扣"
//
// A 4xx upstream response must be:
// 1. Passed through to the client with the exact status and error body.
// 2. NOT retried on other channels (the healthy channel must not be hit).
// 3. Pre-consumed quota must be fully refunded.
// 4. One failed log entry with zero charge.
// 5. Channel status remains enabled (4xx doesn't auto-disable).
func TestRetry6xxNonRetryable4xx(t *testing.T) {
	// Upstream that always returns 400 with a specific error message.
	var badHits int32
	badUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&badHits, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"error":{"message":"max_tokens is too large","type":"invalid_request_error","code":"max_tokens"}}`)
	}))
	t.Cleanup(badUpstream.Close)

	// Healthy upstream that should NEVER be hit for the 4xx model.
	var healthyHits int32
	healthyUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&healthyHits, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"id":"mock","object":"chat.completion","model":"gpt-retry-4xx","choices":[{"index":0,"message":{"role":"assistant","content":"hello"}}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`)
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
	tokenBefore, _ := model.GetTokenByID(env.tokenID)

	// 2. Create two channels: the 4xx channel (higher priority) and a healthy
	//    fallback (lower priority). The 4xx must NOT failover to the healthy one.
	chBad := createChannelID(t, env, map[string]any{
		"name":     "bad-400",
		"key":      upstreamKey,
		"base_url": badUpstream.URL,
		"models":   "gpt-retry-4xx",
		"priority": 30,
	})
	chOK := createChannelID(t, env, map[string]any{
		"name":     "healthy-fallback",
		"key":      upstreamKey,
		"base_url": healthyUpstream.URL,
		"models":   "gpt-retry-4xx",
		"priority": 10,
	})

	// 3. Make the relay request — the 4xx must be passed through.
	chatBody := map[string]any{
		"model":    "gpt-retry-4xx",
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	}
	res = doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, chatBody)

	// 4. Assert: status 400 passed through with exact error body.
	if res.Status != 400 {
		t.Fatalf("4xx should be passed through: got %d, want 400 (body: %s)", res.Status, res.Body)
	}
	if !stringContains(res.Body, "max_tokens is too large") {
		t.Fatalf("error message should be passed through: %s", res.Body)
	}

	// 5. Assert: the bad upstream was hit exactly once.
	if badHits := atomic.LoadInt32(&badHits); badHits != 1 {
		t.Fatalf("bad upstream hits = %d, want 1", badHits)
	}

	// 6. Assert: the healthy fallback was NOT hit (no retry on 4xx).
	if hits := atomic.LoadInt32(&healthyHits); hits != 0 {
		t.Fatalf("healthy fallback hits = %d, want 0 (4xx must not retry on other channels)", hits)
	}

	// 7. Assert: pre-consumed quota fully refunded.
	userAfter, _ := model.GetUserByID(env.rootID)
	if userAfter.Quota != userBefore.Quota {
		t.Fatalf("user quota after 4xx = %d, want %d (fully refunded)",
			userAfter.Quota, userBefore.Quota)
	}
	tokenAfter, _ := model.GetTokenByID(env.tokenID)
	if tokenAfter.RemainQuota != tokenBefore.RemainQuota {
		t.Fatalf("token quota after 4xx = %d, want %d (fully refunded)",
			tokenAfter.RemainQuota, tokenBefore.RemainQuota)
	}

	// 8. Assert: the failed attempt is observable without charging quota.
	logs, total, err := model.ListLogs(env.rootID, 1, 10, "gpt-retry-4xx")
	if err != nil || total != 1 || len(logs) != 1 || logs[0].Status != 2 || logs[0].Quota != 0 || logs[0].ChannelID != chBad || logs[0].RequestedModel != "gpt-retry-4xx" {
		t.Fatalf("4xx relay must write one unbilled log: total=%d logs=%+v err=%v", total, logs, err)
	}

	// 9. Assert: both channels remain enabled (4xx doesn't auto-disable).
	chBadAfter, _ := model.GetChannelByID(chBad)
	if chBadAfter.Status != common.ChannelStatusEnabled {
		t.Fatalf("bad channel status = %d, want %d (enabled) — 4xx must not auto-disable",
			chBadAfter.Status, common.ChannelStatusEnabled)
	}
	chOKAfter, _ := model.GetChannelByID(chOK)
	if chOKAfter.Status != common.ChannelStatusEnabled {
		t.Fatalf("healthy channel status = %d, want %d (enabled)",
			chOKAfter.Status, common.ChannelStatusEnabled)
	}

	// 10. Assert: no channel used quota increase (no consumption).
	if chBadAfter.UsedQuota != 0 {
		t.Fatalf("bad channel used quota = %d, want 0 (no charge on 4xx)", chBadAfter.UsedQuota)
	}
	if chOKAfter.UsedQuota != 0 {
		t.Fatalf("healthy channel used quota = %d, want 0 (was never hit)", chOKAfter.UsedQuota)
	}

	t.Logf("6.2 non-retryable 4xx: status 400 passed through, bad upstream hit=1, healthy hit=0, fully refunded, 1 unbilled log, both channels enabled")
}

// stringContains is a helper to avoid importing strings in this file.
func stringContains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && contains(s, substr))
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

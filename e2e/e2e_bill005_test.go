package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"starhub/model"
	"starhub/relay"
)

// TestBILL005ConcurrentPreConsume verifies that when available quota is
// less than 2× the pre-consume estimate, only one of two concurrent
// requests can pass the pre-consume gate; the other is rejected with
// insufficient_quota and must NOT access the upstream. This is BILL-005.
func TestBILL005ConcurrentPreConsume(t *testing.T) {
	// Slow upstream so concurrent requests hold their pre-payment while
	// others try to reserve theirs. Track upstream hits to verify the
	// rejected request never reached upstream.
	var upstreamHits int32
	upstream := newP2UpstreamWithCounter(t, `{"prompt_tokens":100,"completion_tokens":50}`,
		`{"prompt_tokens":100,"completion_tokens":50}`, 500, &upstreamHits)

	env := setupRelayApp(t)
	createChannelID(t, env, map[string]any{
		"name": "slow-mock", "key": upstreamKey, "base_url": upstream.URL, "models": "gpt-bill-005",
	})

	chatBody := map[string]any{
		"model":      "gpt-bill-005",
		"max_tokens": 1000,
		"messages":   []map[string]string{{"role": "user", "content": "hi"}},
	}
	bodyBytes, _ := json.Marshal(chatBody)
	pre := relay.EstimatePreConsumeQuota("gpt-bill-005", "default", bodyBytes)
	if pre <= 0 {
		t.Fatalf("pre-charge estimate should be positive, got %d", pre)
	}

	// Budget covers exactly 1 full pre-charge plus a tiny surplus that
	// is NOT enough for a second pre-charge. So one request passes,
	// the other is rejected.
	budget := pre + pre/10 // 110% of one pre-charge
	currentUser, _ := model.GetUserByID(env.rootID)
	res := doRequest(env.base, "PUT", "/api/user/", env.session,
		map[string]any{"id": env.rootID, "quota": budget, "expected_quota": currentUser.Quota})
	if res.Status != 200 {
		t.Fatalf("set user quota failed: %d %s", res.Status, res.Body)
	}
	// Create a token with the same limited quota.
	res = doRequest(env.base, "POST", "/api/token/", env.session,
		map[string]any{"name": "bill005", "remain_quota": budget})
	if res.Status != 200 {
		t.Fatalf("create token failed: %d %s", res.Status, res.Body)
	}
	var tokenResp struct {
		Data struct {
			ID  int64  `json:"id"`
			Key string `json:"key"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(res.Body), &tokenResp); err != nil || tokenResp.Data.Key == "" {
		t.Fatalf("bad token response: %s", res.Body)
	}
	limitedKey := tokenResp.Data.Key
	limitedTokenID := tokenResp.Data.ID

	// Barrier: both requests start as close together as possible.
	var barrier sync.WaitGroup
	barrier.Add(2)
	var wg sync.WaitGroup
	wg.Add(2)
	results := make([]apiResult, 2)

	for i := 0; i < 2; i++ {
		go func(idx int) {
			defer wg.Done()
			barrier.Done()
			barrier.Wait()
			results[idx] = doRequest(env.base, "POST", "/v1/chat/completions", limitedKey, chatBody)
		}(i)
	}
	wg.Wait()

	// 1. Exactly one request must succeed (200), the other must be
	//    rejected (403 with insufficient_quota).
	var ok, rejected, other int
	for _, r := range results {
		switch {
		case r.Status == 200:
			ok++
		case r.Status == 403:
			rejected++
		default:
			other++
		}
	}
	if other > 0 {
		t.Fatalf("unexpected statuses: ok=%d rejected=%d other=%d (results: %+v)", ok, rejected, other, results)
	}
	if ok != 1 {
		t.Fatalf("expected exactly 1 successful request, got %d (rejected=%d, pre=%d, budget=%d)",
			ok, rejected, pre, budget)
	}
	if rejected != 1 {
		t.Fatalf("expected exactly 1 rejected request, got %d (ok=%d)", rejected, ok)
	}

	// 2. Upstream must have been hit exactly once — the rejected request
	//    must NOT have reached the upstream.
	hits := atomic.LoadInt32(&upstreamHits)
	if hits != 1 {
		t.Fatalf("upstream hits = %d, want 1 (rejected request must not access upstream)", hits)
	}

	// 3. After settlement, balances must be correct: one successful call
	//    charged the actual usage.
	actual := relay.ComputeQuota("gpt-bill-005", "default", 100, 50)
	token, _ := model.GetTokenByID(limitedTokenID)
	if token.RemainQuota != budget-actual {
		t.Fatalf("token balance after concurrency = %d, want %d (budget=%d - actual=%d)",
			token.RemainQuota, budget-actual, budget, actual)
	}
	user, _ := model.GetUserByID(env.rootID)
	if user.Quota != budget-actual {
		t.Fatalf("user balance after concurrency = %d, want %d (budget=%d - actual=%d)",
			user.Quota, budget-actual, budget, actual)
	}
	if user.Quota < 0 || token.RemainQuota < 0 {
		t.Fatal("balances must never go negative")
	}

	// 4. Exactly one log entry for the successful call.
	if _, total, _ := model.ListLogs(env.rootID, 1, 10, "gpt-bill-005"); total != 1 {
		t.Fatalf("expected 1 success log, got %d rows", total)
	}

	t.Logf("BILL-005: 2 concurrent requests, 1 success 1 rejected, upstream hits=1, charged=%d once, balances correct",
		actual)
}

// newP2UpstreamWithCounter is newP2Upstream with an atomic counter that
// tracks how many times the upstream was actually called.
func newP2UpstreamWithCounter(t *testing.T, usageJSON, streamUsageJSON string, delayMs int64, hits *int32) *httptest.Server {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+upstreamKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		atomic.AddInt32(hits, 1)
		if delayMs > 0 {
			time.Sleep(time.Duration(delayMs) * time.Millisecond)
		}
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Model  string `json:"model"`
			Stream bool   `json:"stream"`
		}
		_ = json.Unmarshal(body, &req)
		if req.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			flusher := w.(http.Flusher)
			chunks := []string{
				fmt.Sprintf(`data: {"id":"mock","choices":[{"index":0,"delta":{"role":"assistant"}}]}`),
				`data: {"id":"mock","choices":[{"index":0,"delta":{"content":"hello "}}]}`,
				`data: {"id":"mock","choices":[{"index":0,"delta":{"content":"world"}}]}`,
				fmt.Sprintf(`data: {"id":"mock","choices":[{"index":0,"delta":{}}],"usage":%s}`, streamUsageJSON),
				`data: [DONE]`,
			}
			for _, c := range chunks {
				fmt.Fprintf(w, "%s\n\n", c)
				flusher.Flush()
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"id":"mock","object":"chat.completion","model":%q,"choices":[{"index":0,"message":{"role":"assistant","content":"hello world"}}],"usage":%s}`, req.Model, usageJSON)
	}))
	t.Cleanup(upstream.Close)
	return upstream
}

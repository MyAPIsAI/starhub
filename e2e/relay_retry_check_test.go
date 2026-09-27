//go:build retrycheck

package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"starhub/common"
	"starhub/model"
)

// Run with: go test -tags=retrycheck ./e2e -run '^TestRetryCheck' -count=1 -v

func retryCheckChat(env p1Env, modelName string) apiResult {
	return doRequest(env.base, http.MethodPost, "/v1/chat/completions", env.apiKey,
		map[string]any{"model": modelName, "messages": []map[string]string{{"role": "user", "content": "ping"}}})
}

func retryCheckChatSuccess(w http.ResponseWriter, modelName string) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"id":"retry-check","object":"chat.completion","model":%q,"choices":[{"index":0,"message":{"role":"assistant","content":"healthy"}}],"usage":{"prompt_tokens":5,"completion_tokens":2}}`, modelName)
}

func TestRetryCheckFiveHundredFailover(t *testing.T) {
	var failedHits, healthyHits atomic.Int32
	failed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		failedHits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(failed.Close)
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		healthyHits.Add(1)
		retryCheckChatSuccess(w, "retry-500")
	}))
	t.Cleanup(healthy.Close)

	env := setupRelayApp(t)
	first := createChannelID(t, env, map[string]any{
		"name": "retry-500-first", "key": upstreamKey, "base_url": failed.URL,
		"models": "retry-500", "priority": 20,
	})
	second := createChannelID(t, env, map[string]any{
		"name": "retry-500-second", "key": upstreamKey, "base_url": healthy.URL,
		"models": "retry-500", "priority": 10,
	})
	res := retryCheckChat(env, "retry-500")
	if res.Status != http.StatusOK || !strings.Contains(res.Body, "healthy") ||
		failedHits.Load() != 1 || healthyHits.Load() != 1 {
		t.Fatalf("status=%d failed_hits=%d healthy_hits=%d body=%s",
			res.Status, failedHits.Load(), healthyHits.Load(), res.Body)
	}
	logs, total, err := model.ListLogs(env.rootID, 1, 10, "retry-500")
	if err != nil || total != 1 || len(logs) != 1 {
		t.Fatalf("usage logs=%d err=%v", total, err)
	}
	if want := fmt.Sprintf("%d->%d", first, second); logs[0].UsedChannels != want {
		t.Fatalf("channel trail=%q, want %q", logs[0].UsedChannels, want)
	}
	t.Logf("5xx failover: hits=1->1 trail=%s", logs[0].UsedChannels)
}

func TestRetryCheckConnectFailureFailover(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	deadURL := "http://" + listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	var healthyHits atomic.Int32
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		healthyHits.Add(1)
		retryCheckChatSuccess(w, "retry-connect")
	}))
	t.Cleanup(healthy.Close)
	env := setupRelayApp(t)
	createChannelID(t, env, map[string]any{
		"name": "retry-connect-first", "key": upstreamKey, "base_url": deadURL,
		"models": "retry-connect", "priority": 20,
	})
	createChannelID(t, env, map[string]any{
		"name": "retry-connect-second", "key": upstreamKey, "base_url": healthy.URL,
		"models": "retry-connect", "priority": 10,
	})
	res := retryCheckChat(env, "retry-connect")
	if res.Status != http.StatusOK || !strings.Contains(res.Body, "healthy") || healthyHits.Load() != 1 {
		t.Fatalf("status=%d healthy_hits=%d body=%s", res.Status, healthyHits.Load(), res.Body)
	}
	t.Logf("connection failure failover: status=%d healthy_hits=%d", res.Status, healthyHits.Load())
}

func TestRetryCheckBudgetAndRefund(t *testing.T) {
	const candidateCount = common.RetryTimes + 3
	var hits [candidateCount]atomic.Int32
	env := setupRelayApp(t)
	for i := range hits {
		i := i
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits[i].Add(1)
			w.WriteHeader(http.StatusInternalServerError)
		}))
		t.Cleanup(upstream.Close)
		createChannelID(t, env, map[string]any{
			"name": fmt.Sprintf("retry-budget-%d", i), "key": upstreamKey,
			"base_url": upstream.URL, "models": "retry-budget", "priority": 100 - i,
		})
	}
	before, err := model.GetTokenByID(env.tokenID)
	if err != nil {
		t.Fatal(err)
	}
	res := retryCheckChat(env, "retry-budget")
	if res.Status != http.StatusBadGateway || !strings.Contains(res.Body, "retry_budget_exhausted") {
		t.Fatalf("budget response=%d %s", res.Status, res.Body)
	}
	var total int32
	for i := range hits {
		got := hits[i].Load()
		total += got
		want := int32(0)
		if i < common.RetryTimes+1 {
			want = 1
		}
		if got != want {
			t.Errorf("channel %d hits=%d, want %d", i, got, want)
		}
	}
	after, err := model.GetTokenByID(env.tokenID)
	if err != nil {
		t.Fatal(err)
	}
	if total != common.RetryTimes+1 || after.RemainQuota != before.RemainQuota {
		t.Errorf("hits=%d quota_before=%d quota_after=%d", total, before.RemainQuota, after.RemainQuota)
	}
	t.Logf("budget exhausted: status=%d hits=%d quota_before=%d quota_after=%d",
		res.Status, total, before.RemainQuota, after.RemainQuota)
}

func TestRetryCheckClientErrorStops(t *testing.T) {
	var badHits, backupHits atomic.Int32
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		badHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"message":"invalid request","type":"invalid_request_error"}}`)
	}))
	t.Cleanup(bad.Close)
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backupHits.Add(1)
		retryCheckChatSuccess(w, "retry-400")
	}))
	t.Cleanup(backup.Close)
	env := setupRelayApp(t)
	createChannelID(t, env, map[string]any{
		"name": "retry-400-first", "key": upstreamKey, "base_url": bad.URL,
		"models": "retry-400", "priority": 20,
	})
	createChannelID(t, env, map[string]any{
		"name": "retry-400-backup", "key": upstreamKey, "base_url": backup.URL,
		"models": "retry-400", "priority": 10,
	})
	before, err := model.GetTokenByID(env.tokenID)
	if err != nil {
		t.Fatal(err)
	}
	res := retryCheckChat(env, "retry-400")
	after, err := model.GetTokenByID(env.tokenID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != http.StatusBadRequest || badHits.Load() != 1 || backupHits.Load() != 0 ||
		after.RemainQuota != before.RemainQuota {
		t.Fatalf("status=%d bad_hits=%d backup_hits=%d quota_before=%d quota_after=%d body=%s",
			res.Status, badHits.Load(), backupHits.Load(), before.RemainQuota, after.RemainQuota, res.Body)
	}
	t.Logf("4xx terminal: status=%d hits=1->0 quota_refunded=true", res.Status)
}

func TestRetryCheckImageTransportUncertain(t *testing.T) {
	var acceptedHits, backupHits atomic.Int32
	accepted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		acceptedHits.Add(1)
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_ = conn.Close()
	}))
	t.Cleanup(accepted.Close)
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backupHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"created":1700000000,"data":[{"url":"https://cdn.example/image.png"}]}`)
	}))
	t.Cleanup(backup.Close)
	env := setupRelayApp(t)
	setOption(t, env, "model_price", `{"gpt-image-1":0.04}`)
	createChannelID(t, env, map[string]any{
		"name": "image-accepted-then-closed", "key": upstreamKey, "base_url": accepted.URL,
		"models": "gpt-image-1", "priority": 20,
	})
	createChannelID(t, env, map[string]any{
		"name": "image-backup", "key": upstreamKey, "base_url": backup.URL,
		"models": "gpt-image-1", "priority": 10,
	})
	res := doRequest(env.base, http.MethodPost, "/v1/images/generations", env.apiKey,
		map[string]any{"model": "gpt-image-1", "prompt": "a cat", "n": 1, "size": "1024x1024"})
	t.Logf("image uncertain: status=%d accepted_hits=%d backup_hits=%d body=%s",
		res.Status, acceptedHits.Load(), backupHits.Load(), res.Body)
	if acceptedHits.Load() != 1 {
		t.Fatalf("first upstream received %d requests, want 1", acceptedHits.Load())
	}
	if backupHits.Load() != 0 || res.Status != http.StatusBadGateway ||
		!strings.Contains(res.Body, "upstream_result_uncertain") {
		t.Errorf("an image request received before disconnect must not be replayed on another channel")
	}
}

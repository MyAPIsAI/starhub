package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"starhub/metric"
	"starhub/service"
	"starhub/setting"
)

// TestP5Observability verifies the metric instrumentation and alerting
// added in P5: request/attempt/channel metrics, TTFT, the /api/metrics
// admin surface, and the webhook notifier on channel disable.
func TestP5Observability(t *testing.T) {
	// A streaming upstream that returns x-request-id so the attempt
	// metric carries the upstream correlation id.
	var streamHits int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "req-upstream-p5")
		if r.URL.Path != "/v1/chat/completions" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Model  string `json:"model"`
			Stream bool   `json:"stream"`
		}
		_ = json.Unmarshal(body, &req)
		atomic.AddInt32(&streamHits, 1)
		if req.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			flusher := w.(http.Flusher)
			chunks := []string{
				`data: {"id":"mock","choices":[{"index":0,"delta":{"role":"assistant"}}]}`,
				`data: {"id":"mock","choices":[{"index":0,"delta":{"content":"hello"}}]}`,
				`data: {"id":"mock","choices":[{"index":0,"delta":{}}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`,
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
		fmt.Fprintf(w, `{"id":"mock","object":"chat.completion","model":%q,"choices":[{"index":0,"message":{"role":"assistant","content":"hi"}}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`, req.Model)
	}))
	t.Cleanup(upstream.Close)

	// A failing upstream (always 500) to exercise the upstream_error path.
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":{"message":"boom","type":"server_error","code":"internal"}}`)
	}))
	t.Cleanup(failing.Close)

	// A webhook sink that records every alert it receives.
	var webhookHits int32
	var lastAlert struct {
		Type      string `json:"type"`
		ChannelID int64  `json:"channel_id"`
		Message   string `json:"message"`
	}
	webhook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&webhookHits, 1)
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &lastAlert)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(webhook.Close)

	env := setupRelayApp(t)
	metric.Reset()
	service.ResetAlertThrottle()

	// Configure the alert webhook so channel-disable alerts are delivered.
	setOption(t, env, setting.OptionKeyAlertWebhookURL, `"`+webhook.URL+`"`)

	// Channel A: healthy, serves gpt-4o-mini.
	channelA := createChannelID(t, env, map[string]any{
		"name": "p5-healthy", "key": upstreamKey, "base_url": upstream.URL,
		"models": "gpt-4o-mini", "group": "default", "priority": 10,
	})

	// 1. Successful non-stream relay records a success metric.
	res := doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey,
		map[string]any{"model": "gpt-4o-mini",
			"messages": []map[string]string{{"role": "user", "content": "hi"}}})
	if res.Status != 200 {
		t.Fatalf("non-stream relay failed: %d %s", res.Status, res.Body)
	}

	// 2. Successful stream relay records a success metric + TTFT.
	res = doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey,
		map[string]any{"model": "gpt-4o-mini", "stream": true,
			"messages": []map[string]string{{"role": "user", "content": "hi"}}})
	if res.Status != 200 || !strings.Contains(res.Body, "hello") {
		t.Fatalf("stream relay failed: %d %s", res.Status, res.Body)
	}

	// 3. The /api/metrics endpoint surfaces the counters and channel stats.
	res = doRequest(env.base, "GET", "/api/metrics", env.session, nil)
	if res.Status != 200 {
		t.Fatalf("metrics endpoint failed: %d %s", res.Status, res.Body)
	}
	var metricsResp struct {
		Data struct {
			Requests []struct {
				Class string `json:"class"`
				Count int64  `json:"count"`
			} `json:"requests"`
			ChannelStats []struct {
				ChannelID   int64   `json:"channel_id"`
				Samples     int     `json:"samples"`
				SuccessRate float64 `json:"success_rate"`
				AvgTTFTMs   float64 `json:"avg_ttft_ms"`
			} `json:"channel_stats"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(res.Body), &metricsResp); err != nil {
		t.Fatalf("bad metrics response: %s", res.Body)
	}
	var successCount int64
	for _, r := range metricsResp.Data.Requests {
		if r.Class == "success" {
			successCount = r.Count
		}
	}
	if successCount < 2 {
		t.Fatalf("success count = %d, want >= 2", successCount)
	}
	// Channel A must have at least 2 samples with 100% success and a
	// positive TTFT (streaming refines it).
	var foundA bool
	for _, cs := range metricsResp.Data.ChannelStats {
		if cs.ChannelID == channelA {
			foundA = true
			if cs.Samples < 2 || cs.SuccessRate != 1.0 {
				t.Fatalf("channel A stats = %+v", cs)
			}
			if cs.AvgTTFTMs <= 0 {
				t.Fatalf("channel A avg ttft = %v, want > 0", cs.AvgTTFTMs)
			}
		}
	}
	if !foundA {
		t.Fatalf("channel A not in metrics: %+v", metricsResp.Data.ChannelStats)
	}

	// 4. A channel that always 500s: every relay attempt fails, the
	// request is classified as upstream_error, and the channel is not
	// auto-disabled (500 is not a disable trigger — only 401/403 and
	// key-error keywords are).
	createChannelID(t, env, map[string]any{
		"name": "p5-failing", "key": upstreamKey, "base_url": failing.URL,
		"models": "gpt-4o-fail", "group": "default",
	})
	res = doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey,
		map[string]any{"model": "gpt-4o-fail",
			"messages": []map[string]string{{"role": "user", "content": "hi"}}})
	if res.Status != 502 {
		t.Fatalf("failing relay should be 502, got %d %s", res.Status, res.Body)
	}

	// Verify the upstream_error request counter incremented.
	res = doRequest(env.base, "GET", "/api/metrics", env.session, nil)
	_ = json.Unmarshal([]byte(res.Body), &metricsResp)
	var upstreamErrCount int64
	for _, r := range metricsResp.Data.Requests {
		if r.Class == "upstream_error" {
			upstreamErrCount = r.Count
		}
	}
	if upstreamErrCount < 1 {
		t.Fatalf("upstream_error count = %d, want >= 1", upstreamErrCount)
	}

	// 5. Alerting: a broken-key channel (401) auto-disables and fires
	// the webhook notifier.
	brokenUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"message":"Incorrect API key provided","type":"invalid_request_error","code":"invalid_api_key"}}`)
	}))
	t.Cleanup(brokenUpstream.Close)

	createChannelID(t, env, map[string]any{
		"name": "p5-broken-key", "key": upstreamKey, "base_url": brokenUpstream.URL,
		"models": "gpt-4o-broken", "group": "default",
	})
	res = doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey,
		map[string]any{"model": "gpt-4o-broken",
			"messages": []map[string]string{{"role": "user", "content": "hi"}}})
	if res.Status != 401 {
		t.Fatalf("broken key relay should pass through 401, got %d %s", res.Status, res.Body)
	}

	// The webhook should have received a channel_disabled alert. The
	// delivery is async, so poll briefly.
	ok := false
	for i := 0; i < 50; i++ {
		if atomic.LoadInt32(&webhookHits) > 0 {
			ok = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !ok {
		t.Fatalf("webhook was not called for channel disable (hits=%d)", atomic.LoadInt32(&webhookHits))
	}
	if lastAlert.Type != "channel_disabled" {
		t.Fatalf("alert type = %q, want channel_disabled", lastAlert.Type)
	}
}

// TestP5ErrorRateThreshold verifies the error-rate threshold alert fires
// when a channel's rolling window crosses the configured limit.
func TestP5ErrorRateThreshold(t *testing.T) {
	metric.Reset()
	service.ResetAlertThrottle()

	// Plant enough failing samples to cross a 0.5 threshold.
	span := metric.BeginRequest(metric.RequestMeta{Model: "m", Mode: "chat"})
	for i := 0; i < 6; i++ {
		a := span.BeginAttempt(999)
		span.EndAttempt(a, metric.AttemptUpstreamError, 500)
	}
	for i := 0; i < 4; i++ {
		a := span.BeginAttempt(999)
		span.EndAttempt(a, metric.AttemptSuccess, 200)
	}
	span.EndRequest(200)

	stat := metric.ChannelStatFor(999)
	if stat == nil || stat.SuccessRate >= 0.5 {
		t.Fatalf("expected success rate < 0.5, got %+v", stat)
	}

	// Configure the threshold and a webhook sink.
	var hits int32
	webhook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(webhook.Close)

	env := setupRelayApp(t)
	setOption(t, env, setting.OptionKeyAlertWebhookURL, `"`+webhook.URL+`"`)
	setOption(t, env, setting.OptionKeyAlertErrorRate, `0.5`)

	// Re-plant the metric (setupRelayApp reset it via ResetAllToDefaults
	// path does not touch metric, but the fresh DB may have triggered
	// other resets).
	metric.Reset()
	span = metric.BeginRequest(metric.RequestMeta{Model: "m", Mode: "chat"})
	for i := 0; i < 6; i++ {
		a := span.BeginAttempt(999)
		span.EndAttempt(a, metric.AttemptUpstreamError, 500)
	}
	for i := 0; i < 4; i++ {
		a := span.BeginAttempt(999)
		span.EndAttempt(a, metric.AttemptSuccess, 200)
	}
	span.EndRequest(200)

	service.CheckErrorRateThresholds()

	ok := false
	for i := 0; i < 50; i++ {
		if atomic.LoadInt32(&hits) > 0 {
			ok = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !ok {
		t.Fatalf("error-rate threshold alert was not delivered (hits=%d)", atomic.LoadInt32(&hits))
	}
}

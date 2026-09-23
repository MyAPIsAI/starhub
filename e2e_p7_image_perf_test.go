package main

// P7 image relay performance measurement (results recorded in
// doc/require/p7图片测试.md). Measures: generation capability (JSON +
// SSE), gateway latency percentiles by payload size, SSE round-trip with
// upstream think time, memory footprint, and concurrent throughput. The
// upstream is a deterministic mock so the numbers isolate gateway
// overhead rather than any real model inference time.

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sort"
	"sync"
	"testing"
	"time"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
)

// p7Payload returns deterministic raw bytes of n bytes (fake PNG data).
func p7Payload(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i*31 + 7)
	}
	return b
}

// p7ImageUpstream serves /v1/images/generations. The returned b64 payload
// size is derived from the requested "size" field; delay controls the
// artificial upstream think time before the first byte (SSE test).
func p7ImageUpstream(t *testing.T, delay time.Duration) *httptest.Server {
	t.Helper()
	sizes := map[string]int{
		"512x512":  200 * 1024,
		"1024x1024": 800 * 1024,
		"1536x1024": 1500 * 1024,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/generations" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Model  string `json:"model"`
			Prompt string `json:"prompt"`
			N      int    `json:"n"`
			Size   string `json:"size"`
			Stream bool   `json:"stream"`
		}
		_ = json.Unmarshal(body, &req)
		if req.N == 0 {
			req.N = 1
		}
		n := sizes[req.Size]
		if n == 0 {
			n = 800 * 1024
		}
		if delay > 0 {
			time.Sleep(delay)
		}
		b64 := base64.StdEncoding.EncodeToString(p7Payload(n))
		if !req.Stream {
			w.Header().Set("Content-Type", "application/json")
			var items []map[string]string
			for i := 0; i < req.N; i++ {
				items = append(items, map[string]string{"b64_json": b64})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": items})
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		for i := 0; i < req.N; i++ {
			fmt.Fprintf(w, "event: image_generation.partial_image\ndata: {\"type\":\"image_generation.partial_image\",\"output_index\":%d,\"b64_json\":\"%s\"}\n\n", i, b64[:4096])
			flusher.Flush()
			fmt.Fprintf(w, "event: image_generation.completed\ndata: {\"type\":\"image_generation.completed\",\"output_index\":%d,\"b64_json\":\"%s\"}\n\n", i, b64)
			flusher.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	t.Cleanup(srv.Close)
	return srv
}

// p7PerfEnv is a relay app with raised rate limits and a big-quota token.
func p7PerfEnv(t *testing.T, upstreamURL string) (base, apiKey string) {
	t.Helper()
	env := setupRelayApp(t)
	// Raise every limiter so throughput runs are not throttled; cleanup
	// restores the compiled defaults (<=0 / <0 are treated as "default").
	middleware.ConfigureRateLimits(1_000_000, 1_000_000)
	middleware.ConfigureTokenRateLimit(1_000_000)
	middleware.ConfigureModelRateLimit(1_000_000)
	t.Cleanup(func() {
		middleware.ConfigureRateLimits(0, 0)
		middleware.ConfigureTokenRateLimit(-1)
		middleware.ConfigureModelRateLimit(-1)
	})
	createChannelID(t, env, map[string]any{
		"name": "p7-image", "key": upstreamKey, "base_url": upstreamURL,
		"models": "gpt-image-1",
	})
	setOption(t, env, "model_price", `{"gpt-image-1":0.001}`)
	res := doRequest(env.base, "POST", "/api/token/", env.session, map[string]any{
		"name": "perf", "remain_quota": 100_000_000,
	})
	if res.Status != 200 {
		t.Fatalf("create perf token failed: %d %s", res.Status, res.Body)
	}
	var tokenResp struct {
		Data struct {
			Key string `json:"key"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(res.Body), &tokenResp); err != nil || tokenResp.Data.Key == "" {
		t.Fatalf("bad perf token response: %s", res.Body)
	}
	return env.base, tokenResp.Data.Key
}

// p7TimedPost performs one relay request and returns (status, duration,
// response bytes).
func p7TimedPost(base, apiKey string, payload any) (int, time.Duration, int) {
	b, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPost, base+"/v1/images/generations", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return -1, time.Since(start), 0
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, time.Since(start), len(raw)
}

func p7Percentiles(durs []time.Duration) (p50, p95, p99, maxD time.Duration) {
	sorted := append([]time.Duration(nil), durs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	pick := func(q float64) time.Duration { return sorted[int(float64(len(sorted)-1)*q)] }
	return pick(0.50), pick(0.95), pick(0.99), sorted[len(sorted)-1]
}

// --- 1. Capability smoke: can the gateway relay generated images? ---

func TestP7ImagePerfSmoke(t *testing.T) {
	upstream := p7ImageUpstream(t, 0)
	base, apiKey := p7PerfEnv(t, upstream.URL)

	// JSON path: n=1 at 1024x1024 (~800KB raw per image, ~1.07MB b64).
	st, dur, bodyLen := p7TimedPost(base, apiKey, map[string]any{
		"model": "gpt-image-1", "prompt": "a cat", "n": 1, "size": "1024x1024",
	})
	if st != 200 {
		t.Fatalf("JSON generation failed: %d", st)
	}
	t.Logf("SMOKE json n=1: status=%d dur=%v bodyBytes=%d", st, dur, bodyLen)

	// JSON path: n=2 (two images in one response).
	st, dur, bodyLen = p7TimedPost(base, apiKey, map[string]any{
		"model": "gpt-image-1", "prompt": "a cat", "n": 2, "size": "1024x1024",
	})
	if st != 200 {
		t.Fatalf("JSON n=2 generation failed: %d", st)
	}
	t.Logf("SMOKE json n=2: status=%d dur=%v bodyBytes=%d", st, dur, bodyLen)

	// SSE path: stream=true.
	st, dur, bodyLen = p7TimedPost(base, apiKey, map[string]any{
		"model": "gpt-image-1", "prompt": "a cat", "n": 1, "size": "1024x1024", "stream": true,
	})
	if st != 200 {
		t.Fatalf("SSE generation failed: %d", st)
	}
	t.Logf("SMOKE sse n=1: status=%d dur=%v bodyBytes=%d", st, dur, bodyLen)

	// Billing: 4 images settled at $0.001 each = 4 x 500 quota.
	perfQuota := func() (remain, used int64) {
		tokens, _ := model.ListTokens(0)
		for _, tok := range tokens {
			if tok.Name == "perf" {
				return tok.RemainQuota, tok.UsedQuota
			}
		}
		return -1, -1
	}
	remain, used := perfQuota()
	spent := 100_000_000 - remain
	t.Logf("SMOKE billing: spent=%d usedQuota=%d (expect spent=%d)", spent, used, 4*int64(0.001*float64(common.QuotaPerUSD)))
}

// --- 2. Latency by payload size (gateway overhead; upstream instant) ---

func TestP7ImagePerfLatency(t *testing.T) {
	if testing.Short() {
		t.Skip("perf measurement skipped in -short mode")
	}
	upstream := p7ImageUpstream(t, 0)
	base, apiKey := p7PerfEnv(t, upstream.URL)
	const iters = 40

	run := func(label string, payload map[string]any) {
		p7TimedPost(base, apiKey, payload) // warmup, excluded
		var durs []time.Duration
		bodyLen := 0
		for i := 0; i < iters; i++ {
			st, d, bl := p7TimedPost(base, apiKey, payload)
			if st != 200 {
				t.Fatalf("%s iter %d failed: %d", label, i, st)
			}
			durs = append(durs, d)
			bodyLen = bl
		}
		p50, p95, p99, maxD := p7Percentiles(durs)
		t.Logf("LATENCY %s bodyBytes=%d iters=%d p50=%v p95=%v p99=%v max=%v",
			label, bodyLen, iters, p50, p95, p99, maxD)
	}

	run("size=512x512", map[string]any{"model": "gpt-image-1", "prompt": "a cat", "n": 1, "size": "512x512"})
	run("size=1024x1024", map[string]any{"model": "gpt-image-1", "prompt": "a cat", "n": 1, "size": "1024x1024"})
	run("size=1536x1024", map[string]any{"model": "gpt-image-1", "prompt": "a cat", "n": 1, "size": "1536x1024"})
	run("n=2@1024x1024", map[string]any{"model": "gpt-image-1", "prompt": "a cat", "n": 2, "size": "1024x1024"})
	run("stream@1024x1024", map[string]any{"model": "gpt-image-1", "prompt": "a cat", "n": 1, "size": "1024x1024", "stream": true})
}

// --- 3. SSE round-trip with a simulated upstream think time ---

func TestP7ImagePerfStreamWithDelay(t *testing.T) {
	if testing.Short() {
		t.Skip("perf measurement skipped in -short mode")
	}
	upstream := p7ImageUpstream(t, 500*time.Millisecond)
	base, apiKey := p7PerfEnv(t, upstream.URL)
	const iters = 15

	p7TimedPost(base, apiKey, map[string]any{ // warmup
		"model": "gpt-image-1", "prompt": "a cat", "n": 1, "size": "1024x1024", "stream": true,
	})
	var durs []time.Duration
	for i := 0; i < iters; i++ {
		st, d, _ := p7TimedPost(base, apiKey, map[string]any{
			"model": "gpt-image-1", "prompt": "a cat", "n": 1, "size": "1024x1024", "stream": true,
		})
		if st != 200 {
			t.Fatalf("sse iter %d failed: %d", i, st)
		}
		durs = append(durs, d)
	}
	p50, p95, p99, maxD := p7Percentiles(durs)
	t.Logf("SSE-DELAY upstreamDelay=500ms iters=%d p50=%v p95=%v p99=%v max=%v",
		iters, p50, p95, p99, maxD)
}

// --- 4. Memory footprint: 30 requests x ~800KB images ---

func TestP7ImagePerfMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("perf measurement skipped in -short mode")
	}
	upstream := p7ImageUpstream(t, 0)
	base, apiKey := p7PerfEnv(t, upstream.URL)

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	var peakHeap uint64
	const iters = 30
	for i := 0; i < iters; i++ {
		st, _, _ := p7TimedPost(base, apiKey, map[string]any{
			"model": "gpt-image-1", "prompt": "a cat", "n": 1, "size": "1024x1024",
		})
		if st != 200 {
			t.Fatalf("mem iter %d failed: %d", i, st)
		}
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		if ms.HeapAlloc > peakHeap {
			peakHeap = ms.HeapAlloc
		}
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	t.Logf("MEMORY iters=%d heapAllocBefore=%.1fMB heapAllocAfter=%.1fMB heapInuseDelta=%+.1fMB totalAllocDelta=%+.1fMB sysDelta=%+.1fMB peakHeap=%.1fMB numGC=%d mallocsDelta=%d",
		iters,
		float64(before.HeapAlloc)/1e6, float64(after.HeapAlloc)/1e6,
		float64(int64(after.HeapInuse)-int64(before.HeapInuse))/1e6,
		float64(after.TotalAlloc-before.TotalAlloc)/1e6,
		float64(int64(after.Sys)-int64(before.Sys))/1e6,
		float64(peakHeap)/1e6,
		after.NumGC-before.NumGC,
		after.Mallocs-before.Mallocs)
}

// --- 5. Concurrent throughput ---

func TestP7ImagePerfConcurrency(t *testing.T) {
	if testing.Short() {
		t.Skip("perf measurement skipped in -short mode")
	}
	upstream := p7ImageUpstream(t, 0)
	base, apiKey := p7PerfEnv(t, upstream.URL)
	const workers = 24
	const perWorker = 3

	var wg sync.WaitGroup
	results := make([]int, workers*perWorker)
	start := time.Now()
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(off int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				st, _, _ := p7TimedPost(base, apiKey, map[string]any{
					"model": "gpt-image-1", "prompt": "a cat", "n": 1, "size": "1024x1024",
				})
				results[off+i] = st
			}
		}(w * perWorker)
	}
	wg.Wait()
	wall := time.Since(start)
	ok := 0
	for _, st := range results {
		if st == 200 {
			ok++
		}
	}
	total := workers * perWorker
	t.Logf("CONCURRENCY workers=%d perWorker=%d total=%d ok=%d wall=%v throughput=%.1f req/s",
		workers, perWorker, total, ok, wall, float64(total)/wall.Seconds())
}

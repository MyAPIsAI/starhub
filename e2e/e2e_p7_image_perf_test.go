package main

// P7 image relay performance measurement. Measures: capability (JSON +
// SSE), gateway latency percentiles by payload size, SSE round-trip with
// upstream think time, memory footprint, and concurrent throughput.
// Upstream payloads are prepared before measurements. Paired direct and
// relay requests help distinguish fixture costs from gateway overhead.

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
		"512x512":   200 * 1024,
		"1024x1024": 800 * 1024,
		"1536x1024": 1500 * 1024,
	}
	payloads := make(map[string][]byte, len(sizes))
	for size, n := range sizes {
		payloads[size] = []byte(base64.StdEncoding.EncodeToString(p7Payload(n)))
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
		b64 := payloads[req.Size]
		if b64 == nil {
			b64 = payloads["1024x1024"]
		}
		if delay > 0 {
			time.Sleep(delay)
		}
		if !req.Stream {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":[`)
			for i := 0; i < req.N; i++ {
				if i > 0 {
					_, _ = io.WriteString(w, ",")
				}
				_, _ = io.WriteString(w, `{"b64_json":"`)
				_, _ = w.Write(b64)
				_, _ = io.WriteString(w, `"}`)
			}
			_, _ = io.WriteString(w, "]}\n")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		for i := 0; i < req.N; i++ {
			fmt.Fprintf(w, "event: image_generation.partial_image\ndata: {\"type\":\"image_generation.partial_image\",\"output_index\":%d,\"b64_json\":\"", i)
			_, _ = w.Write(b64[:4096])
			_, _ = io.WriteString(w, "\"}\n\n")
			flusher.Flush()
			fmt.Fprintf(w, "event: image_generation.completed\ndata: {\"type\":\"image_generation.completed\",\"output_index\":%d,\"b64_json\":\"", i)
			_, _ = w.Write(b64)
			_, _ = io.WriteString(w, "\"}\n\n")
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
	n, err := io.Copy(io.Discard, resp.Body)
	if err != nil {
		return -1, time.Since(start), int(n)
	}
	return resp.StatusCode, time.Since(start), int(n)
}

func p7Percentiles(durs []time.Duration) (p50, p95, p99, maxD time.Duration) {
	sorted := append([]time.Duration(nil), durs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	pick := func(q float64) time.Duration { return sorted[int(float64(len(sorted)-1)*q)] }
	return pick(0.50), pick(0.95), pick(0.99), sorted[len(sorted)-1]
}

// --- 1. Capability smoke: can the gateway relay generated images? ---

// p7CheckedPost verifies complete wire content outside the allocation probe.
func p7CheckedPost(t *testing.T, upstream, base, apiKey string, payload any) (int, time.Duration, int) {
	t.Helper()
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	read := func(target string) (int, []byte) {
		req, err := http.NewRequest(http.MethodPost, target+"/v1/images/generations", bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, raw
	}
	wantStatus, want := read(upstream)
	start := time.Now()
	status, got := read(base)
	duration := time.Since(start)
	if status != wantStatus || !bytes.Equal(got, want) {
		t.Fatalf("image response differs: direct status=%d bytes=%d, relay status=%d bytes=%d", wantStatus, len(want), status, len(got))
	}
	return status, duration, len(got)
}

func TestP7ImagePerfSmoke(t *testing.T) {
	upstream := p7ImageUpstream(t, 0)
	base, apiKey := p7PerfEnv(t, upstream.URL)

	// JSON path: n=1 at 1024x1024 (~800KB raw per image, ~1.07MB b64).
	st, dur, bodyLen := p7CheckedPost(t, upstream.URL, base, apiKey, map[string]any{
		"model": "gpt-image-1", "prompt": "a cat", "n": 1, "size": "1024x1024",
	})
	if st != 200 {
		t.Fatalf("JSON generation failed: %d", st)
	}
	t.Logf("SMOKE json n=1: status=%d dur=%v bodyBytes=%d", st, dur, bodyLen)

	// JSON path: n=2 (two images in one response).
	st, dur, bodyLen = p7CheckedPost(t, upstream.URL, base, apiKey, map[string]any{
		"model": "gpt-image-1", "prompt": "a cat", "n": 2, "size": "1024x1024",
	})
	if st != 200 {
		t.Fatalf("JSON n=2 generation failed: %d", st)
	}
	t.Logf("SMOKE json n=2: status=%d dur=%v bodyBytes=%d", st, dur, bodyLen)

	// SSE path: stream=true.
	st, dur, bodyLen = p7CheckedPost(t, upstream.URL, base, apiKey, map[string]any{
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
	want := 4 * int64(0.001*float64(common.QuotaPerUSD))
	deadline := time.Now().Add(2 * time.Second)
	for used != want && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
		remain, used = perfQuota()
	}
	spent := 100_000_000 - remain
	if spent != want || used != want {
		t.Fatalf("billing: spent=%d used=%d want=%d", spent, used, want)
	}
	t.Logf("SMOKE billing: spent=%d usedQuota=%d", spent, used)
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
	for _, stream := range []bool{false, true} {
		payload := map[string]any{
			"model": "gpt-image-1", "prompt": "a cat", "n": 1, "size": "1024x1024", "stream": stream,
		}
		direct := p7MeasureMemory(t, "direct", stream, upstream.URL, apiKey, payload)
		relay := p7MeasureMemory(t, "relay", stream, base, apiKey, payload)
		t.Logf("MEMORY-PAIR stream=%t approximateRelayAlloc=%.1fKiB/request (same-process delta, not peak memory)",
			stream, float64(int64(relay)-int64(direct))/30/1024)
	}
}

func p7MeasureMemory(t *testing.T, route string, stream bool, base, apiKey string, payload any) uint64 {
	t.Helper()
	for range 2 {
		if st, _, _ := p7TimedPost(base, apiKey, payload); st != http.StatusOK {
			t.Fatalf("%s warmup failed: %d", route, st)
		}
	}
	// Sample throughout requests. This is a sampled process heap, not an
	// exact peak or the gateway's private working set.
	stop := make(chan struct{})
	peak := make(chan uint64, 1)
	go func() {
		ticker := time.NewTicker(2 * time.Millisecond)
		defer ticker.Stop()
		var high uint64
		for {
			select {
			case <-ticker.C:
				var ms runtime.MemStats
				runtime.ReadMemStats(&ms)
				high = max(high, ms.HeapAlloc)
			case <-stop:
				peak <- high
				return
			}
		}
	}()
	defer close(stop)
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	const iters = 30
	bodyLen := 0
	for i := 0; i < iters; i++ {
		st, _, n := p7TimedPost(base, apiKey, payload)
		if st != 200 {
			t.Fatalf("mem iter %d failed: %d", i, st)
		}
		if n == 0 || (i > 0 && n != bodyLen) {
			t.Fatalf("response size changed: got=%d previous=%d", n, bodyLen)
		}
		bodyLen = n
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	stop <- struct{}{}
	peakHeap := max(<-peak, before.HeapAlloc, after.HeapAlloc)
	t.Logf("MEMORY route=%s stream=%t iters=%d bodyBytes=%d heapAllocBefore=%.1fMB heapAllocAfter=%.1fMB heapInuseDelta=%+.1fMB totalAllocDelta=%+.1fMB sysDelta=%+.1fMB sampledPeakHeap=%.1fMB numGC=%d mallocsDelta=%d",
		route, stream, iters, bodyLen,
		float64(before.HeapAlloc)/1e6, float64(after.HeapAlloc)/1e6,
		float64(int64(after.HeapInuse)-int64(before.HeapInuse))/1e6,
		float64(after.TotalAlloc-before.TotalAlloc)/1e6,
		float64(int64(after.Sys)-int64(before.Sys))/1e6,
		float64(peakHeap)/1e6,
		after.NumGC-before.NumGC,
		after.Mallocs-before.Mallocs)
	return after.TotalAlloc - before.TotalAlloc
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
	if ok != total {
		t.Fatalf("concurrent requests: %d/%d succeeded", ok, total)
	}
	t.Logf("CONCURRENCY workers=%d perWorker=%d total=%d ok=%d wall=%v throughput=%.1f req/s",
		workers, perWorker, total, ok, wall, float64(total)/wall.Seconds())
}

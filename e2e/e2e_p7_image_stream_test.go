package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestP7ImagePerfStreamFirstEvent(t *testing.T) {
	if testing.Short() {
		t.Skip("perf measurement skipped in -short mode")
	}
	b64 := base64.StdEncoding.EncodeToString(p7Payload(800 * 1024))
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/generations" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var request struct {
			Stream bool `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || !request.Stream {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		time.Sleep(50 * time.Millisecond)
		fmt.Fprint(w, "event: image_generation.partial_image\ndata: {\"type\":\"image_generation.partial_image\",\"output_index\":0,\"b64_json\":\"")
		io.WriteString(w, b64[:4096])
		fmt.Fprint(w, "\"}\n\n")
		w.(http.Flusher).Flush()
		time.Sleep(80 * time.Millisecond)
		fmt.Fprint(w, "event: image_generation.completed\ndata: {\"type\":\"image_generation.completed\",\"output_index\":0,\"b64_json\":\"")
		io.WriteString(w, b64)
		fmt.Fprint(w, "\"}\n\ndata: [DONE]\n\n")
		w.(http.Flusher).Flush()
	}))
	t.Cleanup(upstream.Close)
	base, apiKey := p7PerfEnv(t, upstream.URL)
	payload, err := json.Marshal(map[string]any{
		"model": "gpt-image-1", "prompt": "a cat", "n": 1, "size": "1024x1024", "stream": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	probe := func(target string) (time.Duration, time.Duration) {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, target+"/v1/images/generations", bytes.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)
		start := time.Now()
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("stream status = %d", resp.StatusCode)
		}
		reader := bufio.NewReader(resp.Body)
		var first time.Duration
		var completed, done bool
		for {
			line, readErr := reader.ReadString('\n')
			if strings.HasPrefix(line, "event: image_generation.partial_image") && first == 0 {
				first = time.Since(start)
			}
			if strings.HasPrefix(line, "event: image_generation.completed") {
				completed = true
			}
			if strings.HasPrefix(line, "data: [DONE]") {
				done = true
			}
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				t.Fatal(readErr)
			}
		}
		total := time.Since(start)
		if first == 0 || !completed || !done {
			t.Fatalf("stream response missing events: first=%v total=%v completed=%t done=%t", first, total, completed, done)
		}
		return first, total
	}
	const iters = 10
	for _, route := range []struct {
		name string
		url  string
	}{{"direct", upstream.URL}, {"relay", base}} {
		firsts := make([]time.Duration, 0, iters)
		totals := make([]time.Duration, 0, iters)
		early := 0
		for i := 0; i < iters; i++ {
			first, total := probe(route.url)
			firsts = append(firsts, first)
			totals = append(totals, total)
			if total-first >= 50*time.Millisecond {
				early++
			}
		}
		firstP50, firstP95, _, _ := p7Percentiles(firsts)
		totalP50, totalP95, _, _ := p7Percentiles(totals)
		t.Logf("FIRST-EVENT route=%s iters=%d early=%d firstP50=%v firstP95=%v totalP50=%v totalP95=%v", route.name, iters, early, firstP50, firstP95, totalP50, totalP95)
	}
}

func TestP7ImagePerfStreamConcurrency(t *testing.T) {
	if testing.Short() {
		t.Skip("perf measurement skipped in -short mode")
	}
	upstream := p7ImageUpstream(t, 0)
	base, apiKey := p7PerfEnv(t, upstream.URL)
	const workers = 24
	const perWorker = 3
	results := make([]int, workers*perWorker)
	var wg sync.WaitGroup
	start := time.Now()
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(off int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				status, _, bodyBytes := p7TimedPost(base, apiKey, map[string]any{
					"model": "gpt-image-1", "prompt": "a cat", "n": 1, "size": "1024x1024", "stream": true,
				})
				if status == http.StatusOK && bodyBytes > 0 {
					results[off+i] = status
				}
			}
		}(w * perWorker)
	}
	wg.Wait()
	wall := time.Since(start)
	ok := 0
	for _, status := range results {
		if status == http.StatusOK {
			ok++
		}
	}
	if ok != len(results) {
		t.Fatalf("concurrent stream requests: %d/%d succeeded", ok, len(results))
	}
	t.Logf("STREAM-CONCURRENCY workers=%d perWorker=%d total=%d ok=%d wall=%v throughput=%.1f req/s", workers, perWorker, len(results), ok, wall, float64(len(results))/wall.Seconds())
}

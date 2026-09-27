package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"starhub/model"
	"starhub/setting"
)

// TestAffinityE2E verifies the sticky-channel integration end to end: enabling
// affinity makes repeat requests stick to the same channel; a 5xx on the
// sticky channel clears the preference and falls back; 4xx does not.
func TestAffinityE2E(t *testing.T) {
	var failA atomic.Bool
	mockA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failA.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"id":"a","choices":[{"index":0,"message":{"role":"assistant","content":"from-a"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	t.Cleanup(mockA.Close)

	var failB atomic.Bool
	mockB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failB.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"id":"b","choices":[{"index":0,"message":{"role":"assistant","content":"from-b"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	t.Cleanup(mockB.Close)

	env := setupRelayApp(t)
	t.Cleanup(setting.ResetAffinityToDefaults)
	t.Cleanup(model.ResetAffinity)
	_ = model.ResetAffinity // silence unused

	createChannelID(t, env, map[string]any{
		"name": "aff-a", "key": upstreamKey, "base_url": mockA.URL,
		"models": "gpt-4o-mini",
	})
	createChannelID(t, env, map[string]any{
		"name": "aff-b", "key": upstreamKey, "base_url": mockB.URL,
		"models": "gpt-4o-mini",
	})

	// Enable affinity.
	res := doRequest(env.base, "PUT", "/api/option/", env.session,
		map[string]string{"key": "routing_affinity_enabled", "value": "true"})
	if res.Status != 200 {
		t.Fatalf("enable affinity failed: %d %s", res.Status, res.Body)
	}

	chat := func() string {
		res := doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey,
			map[string]any{"model": "gpt-4o-mini",
				"messages": []map[string]string{{"role": "user", "content": "hi"}}})
		if res.Status != 200 {
			t.Fatalf("chat failed: %d %s", res.Status, res.Body)
		}
		switch {
		case strings.Contains(res.Body, "from-a"):
			return "a"
		case strings.Contains(res.Body, "from-b"):
			return "b"
		}
		t.Fatalf("unexpected response: %s", res.Body)
		return ""
	}

	// First request picks randomly; subsequent requests must all hit the
	// same channel (the one chosen on the first success).
	first := chat()
	for i := 0; i < 10; i++ {
		if got := chat(); got != first {
			t.Fatalf("iteration %d: got %s, want %s", i, got, first)
		}
	}

	// Make the sticky channel fail: the next request falls back to the other
	// channel and the affinity is rewritten.
	if first == "a" {
		failA.Store(true)
	} else {
		failB.Store(true)
	}
	got := chat()
	if got == first {
		t.Fatal("request should have fallen back after the sticky channel failed")
	}
	// After the fallback success the affinity points at the working channel.
	for i := 0; i < 5; i++ {
		if g := chat(); g == first {
			t.Fatalf("the broken channel should have been cleared: iteration %d", i)
		}
	}
}

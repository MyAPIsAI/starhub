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

	"starhub/model"
	"starhub/service"
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

func TestStrictAffinityHTTPRefund(t *testing.T) {
	env := setupRelayApp(t)
	t.Cleanup(setting.ResetAffinityToDefaults)
	t.Cleanup(model.ResetAffinity)
	var calls atomic.Int64
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) > 1 {
			http.Error(w, "fixture failure", 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"a","choices":[{"index":0,"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":10,"completion_tokens":10}}`)
	}))
	t.Cleanup(s.Close)
	createChannelID(t, env, map[string]any{"name": "strict", "key": upstreamKey, "base_url": s.URL, "models": "gpt-4o-mini"})
	for key, value := range map[string]string{"routing_affinity_enabled": "true", "routing_affinity_rules": `[{"name":"strict","model_regex":[".*"],"key_sources":[{"type":"gjson","path":"user"}],"session_mode":"strict"}]`} {
		res := doRequest(env.base, "PUT", "/api/option/", env.session, map[string]string{"key": key, "value": value})
		if res.Status != 200 {
			t.Fatal(res.Body)
		}
	}
	balance := func() [2]int64 {
		u, err := model.GetUserByID(env.rootID)
		if err != nil {
			t.Fatal(err)
		}
		tok, err := model.GetTokenByID(env.tokenID)
		if err != nil {
			t.Fatal(err)
		}
		return [2]int64{u.Quota, tok.RemainQuota}
	}
	chat := func() apiResult {
		return doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, map[string]any{"model": "gpt-4o-mini", "user": "same", "messages": []map[string]string{{"role": "user", "content": "hi"}}})
	}
	before := balance()
	if res := chat(); res.Status != 200 {
		t.Fatal(res.Body)
	}
	paid := balance()
	if paid == before {
		t.Fatal("successful request did not settle")
	}
	for range 2 {
		if res := chat(); res.Status != 503 || !strings.Contains(res.Body, "strict_affinity_unavailable") {
			t.Fatalf("unexpected response: %d %s", res.Status, res.Body)
		}
		if balance() != paid || calls.Load() != 2 {
			t.Fatal("failed request changed balance or sent an extra attempt")
		}
	}
}

func TestPlaygroundHeaderAffinityDiagnostics(t *testing.T) {
	env := setupRelayApp(t)
	model.ResetAffinity()
	t.Cleanup(model.ResetAffinity)
	t.Cleanup(setting.ResetAffinityToDefaults)
	up := newMockUpstream(t)
	createChannelID(t, env, map[string]any{"name": "diagnostics", "type": 1, "key": upstreamKey, "base_url": up.URL, "models": "gpt-4o-mini"})
	for key, value := range map[string]string{"routing_affinity_enabled": "true", "routing_affinity_rules": `[{"name":"diagnostic-rule","model_regex":["^gpt-4o-mini$"],"path_regex":["^/v1/chat/completions$"],"key_sources":[{"type":"request_header","key":"X-Session-Id"}]}]`} {
		r := doRequest(env.base, "PUT", "/api/option", env.session, map[string]string{"key": key, "value": value})
		if r.Status != 200 {
			t.Fatal(r.Body)
		}
	}
	path := fmt.Sprintf("/api/playground/%d", env.tokenID)
	unlock := doRequest(env.base, "POST", path+"/unlock", env.session, map[string]any{"expected_user_id": env.rootID, "password": "123456"})
	if unlock.Status != 200 {
		t.Fatal(unlock.Body)
	}
	stats := func() service.AffinityCacheStats {
		r := doRequest(env.base, "GET", "/api/option/routing_affinity_cache", env.session, nil)
		var s struct{ Data service.AffinityCacheStats }
		if r.Status != 200 || json.Unmarshal([]byte(r.Body), &s) != nil || s.Data.Instance == "" {
			t.Fatalf("invalid statistics: %s", r.Body)
		}
		return s.Data
	}
	before := stats()
	for i, want := range []int{0, 0, 1, 1} {
		r, err := http.NewRequest("POST", env.base+path+"/chat/completions", strings.NewReader(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}`))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Content-Type", "application/json")
		r.AddCookie(&http.Cookie{Name: "session", Value: env.session})
		if i >= 2 {
			r.Header.Set("X-Session-Id", "diagnostic-fixture")
		}
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || resp.StatusCode != 200 || resp.Header.Get("X-StarHub-Affinity-Instance") != before.Instance {
			t.Fatalf("unexpected relay: %d %s", resp.StatusCode, body)
		}
		after := stats()
		if after.Total != want || after.ByRuleName["diagnostic-rule"] != want {
			t.Fatalf("attempt %d: %+v", i, after)
		}
	}
	after := stats()
	detail := doRequest(env.base, "GET", "/api/option/routing_affinity_cache?rule=diagnostic-rule&page=1", env.session, nil)
	var details struct{ Data service.AffinityBindingPage }
	if detail.Status != 200 || json.Unmarshal([]byte(detail.Body), &details) != nil || details.Data.Instance != after.Instance || details.Data.Total != 1 || len(details.Data.Entries) != 1 || details.Data.Entries[0].ChannelID <= 0 || details.Data.Entries[0].ExpiresAt <= details.Data.ObservedAt || strings.Contains(detail.Body, "diagnostic-fixture") {
		t.Fatalf("unexpected rule details: %s", detail.Body)
	}
	for event, want := range map[string]uint64{"missing_source": 2, "legacy_hit": 1, "cold_start": 1, "rule_hit": 1, "write_ok": 2} {
		if delta := after.Events[event] - before.Events[event]; delta != want {
			t.Errorf("%s: %d, want %d", event, delta, want)
		}
	}
}

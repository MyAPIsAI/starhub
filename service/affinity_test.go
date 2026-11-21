package service

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"starhub/setting"
)

func resetAffinityForTest(t *testing.T) {
	t.Helper()
	setting.ResetAffinityToDefaults()
	ResetRuleAffinityCache()
	t.Cleanup(func() {
		setting.ResetAffinityToDefaults()
		ResetRuleAffinityCache()
	})
}

func enableRuleAffinity(t *testing.T, rulesJSON string) {
	t.Helper()
	if err := setting.UpdateOption(setting.OptionKeyRoutingAffinityEnabled, `true`); err != nil {
		t.Fatalf("enable affinity: %v", err)
	}
	if err := setting.UpdateOption(setting.OptionKeyRoutingAffinityRules, rulesJSON); err != nil {
		t.Fatalf("set rules: %v", err)
	}
}

func TestResolveAffinityNoRulesFallsBack(t *testing.T) {
	resetAffinityForTest(t)
	if err := setting.UpdateOption(setting.OptionKeyRoutingAffinityEnabled, `true`); err != nil {
		t.Fatal(err)
	}
	meta := ResolveAffinity(AffinityRequest{ModelName: "gpt-4o"})
	if meta.Matched {
		t.Fatal("no rules configured must not match")
	}
}

func TestResolveAffinityDisabledIgnoresRules(t *testing.T) {
	resetAffinityForTest(t)
	enableRuleAffinity(t, `[{"name":"r","model_regex":[".*"],"key_sources":[{"type":"gjson","path":"session_id"}]}]`)
	// Override the enabled flag back to false.
	if err := setting.UpdateOption(setting.OptionKeyRoutingAffinityEnabled, `false`); err != nil {
		t.Fatal(err)
	}
	meta := ResolveAffinity(AffinityRequest{ModelName: "gpt-4o", Body: []byte(`{"session_id":"s1"}`)})
	if meta.Matched {
		t.Fatal("disabled affinity must not match even with rules")
	}
}

func TestRuleCacheScope(t *testing.T) {
	a := AffinityRequest{TokenID: 1, ModelName: "a:b", UsingGroup: "c"}
	b := a
	b.TokenID++
	c := a
	c.ModelName = "a"
	c.UsingGroup = "b:c"
	r := setting.AffinityRule{Name: "r"}
	k := affinityCacheKey(r, a, "session")
	if k == affinityCacheKey(r, b, "session") || k == affinityCacheKey(r, c, "session") || k == affinityCacheKey(setting.AffinityRule{Name: "s"}, a, "session") {
		t.Fatal("independent cache scopes collided")
	}
}

func TestResolveAffinityMatchesAndMissesCache(t *testing.T) {
	resetAffinityForTest(t)
	enableRuleAffinity(t, `[{"name":"codex","model_regex":["^gpt-5$"],"path_regex":["/v1/responses"],"key_sources":[{"type":"gjson","path":"prompt_cache_key"}],"include_rule_name":true,"session_mode":"strict"}]`)

	// Matching model + path + present key → rule matches, cache miss.
	meta := ResolveAffinity(AffinityRequest{
		ModelName:   "gpt-5",
		RequestPath: "/v1/responses",
		Body:        []byte(`{"prompt_cache_key":"abc123"}`),
	})
	if !meta.Matched {
		t.Fatal("expected rule to match")
	}
	if meta.Found {
		t.Fatal("cache should be empty on first resolve")
	}
	if meta.SessionMode != setting.AffinitySessionStrict {
		t.Fatalf("session mode = %q, want strict", meta.SessionMode)
	}
	if meta.KeyFingerprint == "" {
		t.Fatal("key fingerprint must be populated for logs")
	}

	// Non-matching model → no rule.
	if m := ResolveAffinity(AffinityRequest{ModelName: "claude-3", Body: []byte(`{"prompt_cache_key":"x"}`)}); m.Matched {
		t.Fatal("claude model must not match gpt-5 rule")
	}
	// Matching model but missing key source value → no rule.
	if m := ResolveAffinity(AffinityRequest{ModelName: "gpt-5", RequestPath: "/v1/responses", Body: []byte(`{}`)}); m.Matched {
		t.Fatal("missing affinity value must not match")
	}
}

func TestResolveAffinityHeaderSourceAndCacheHit(t *testing.T) {
	resetAffinityForTest(t)
	enableRuleAffinity(t, `[{"name":"hdr","model_regex":[".*"],"key_sources":[{"type":"request_header","key":"X-Session"}],"include_rule_name":true}]`)

	req := AffinityRequest{
		ModelName: "gpt-4o",
		Header:    http.Header{"X-Session": []string{"sess-1"}},
	}
	meta := ResolveAffinity(req)
	if !meta.Matched || meta.Found {
		t.Fatalf("expected match without cache hit, got matched=%v found=%v", meta.Matched, meta.Found)
	}

	// Record → next resolve hits.
	RecordRuleAffinity(meta, 42)
	if ttl := meta.TTLSeconds; ttl <= 0 {
		t.Fatalf("ttl must be positive, got %d", ttl)
	}
	meta2 := ResolveAffinity(req)
	if !meta2.Found || meta2.ChannelID != 42 {
		t.Fatalf("expected cache hit channel 42, got found=%v channel=%d", meta2.Found, meta2.ChannelID)
	}

	// Clear → miss again.
	ClearRuleAffinity(&meta2)
	meta3 := ResolveAffinity(req)
	if meta3.Found {
		t.Fatal("cache should be empty after clear")
	}
}

func TestResolveAffinitySessionOffSkipsCache(t *testing.T) {
	resetAffinityForTest(t)
	enableRuleAffinity(t, `[{"name":"off","model_regex":[".*"],"key_sources":[{"type":"gjson","path":"k"}],"session_mode":"off"}]`)

	meta := ResolveAffinity(AffinityRequest{ModelName: "x", Body: []byte(`{"k":"v"}`)})
	if !meta.Matched {
		t.Fatal("rule must match")
	}
	if meta.SessionMode != setting.AffinitySessionOff {
		t.Fatalf("mode = %q, want off", meta.SessionMode)
	}
	if meta.CacheKey != "" {
		t.Fatalf("off mode must not build a cache key, got %q", meta.CacheKey)
	}
	RecordRuleAffinity(meta, 7)
	if RuleAffinityStats().Total != 0 {
		t.Fatal("off mode must not record")
	}
}

func TestResolveAffinityTTLExpiry(t *testing.T) {
	resetAffinityForTest(t)
	enableRuleAffinity(t, `[{"name":"r","model_regex":[".*"],"key_sources":[{"type":"gjson","path":"k"}],"ttl_seconds":1,"include_rule_name":true}]`)

	meta := ResolveAffinity(AffinityRequest{ModelName: "x", Body: []byte(`{"k":"v"}`)})
	RecordRuleAffinity(meta, 9)
	// Manually expire the entry.
	for key, entry := range ruleAffinityCache {
		entry.expiresAt = time.Now().Add(-time.Second)
		ruleAffinityCache[key] = entry
	}
	if m := ResolveAffinity(AffinityRequest{ModelName: "x", Body: []byte(`{"k":"v"}`)}); m.Found {
		t.Fatal("expired entry must not be found")
	}
}

func TestRuleAffinityStatsAndClearAll(t *testing.T) {
	resetAffinityForTest(t)
	enableRuleAffinity(t, `[{"name":"r1","model_regex":[".*"],"key_sources":[{"type":"gjson","path":"k"}],"include_rule_name":true}]`)

	meta := ResolveAffinity(AffinityRequest{ModelName: "x", Body: []byte(`{"k":"v1"}`)})
	RecordRuleAffinity(meta, 1)
	meta2 := ResolveAffinity(AffinityRequest{ModelName: "x", Body: []byte(`{"k":"v2"}`)})
	RecordRuleAffinity(meta2, 2)

	stats := RuleAffinityStats()
	if stats.Total != 2 {
		t.Fatalf("stats total = %d, want 2", stats.Total)
	}
	if stats.ByRuleName["r1"] != 2 {
		t.Fatalf("by_rule_name r1 = %d, want 2", stats.ByRuleName["r1"])
	}

	if cleared := ClearAllRuleAffinity(); cleared != 2 {
		t.Fatalf("clear all = %d, want 2", cleared)
	}
	stats2 := RuleAffinityStats()
	if stats2.Total != 0 {
		t.Fatalf("after clear total = %d, want 0", stats2.Total)
	}
}

func TestGjsonPathWalker(t *testing.T) {
	for _, n := range []string{"9007199254740992", "9007199254740993", "18446744073709551615"} {
		if got := affinityGjsonPath([]byte(`{"id":`+n+`}`), "id"); got != n {
			t.Fatalf("lost JSON identity: %s became %s", n, got)
		}
	}
	body := []byte(`{"metadata":{"user_id":"u-42"},"prompt_cache_key":"pk","count":3,"flag":true,"arr":[1,2]}`)
	for _, tc := range []struct {
		path string
		want string
	}{
		{"metadata.user_id", "u-42"},
		{"prompt_cache_key", "pk"},
		{"count", "3"},
		{"flag", "true"},
		{"missing", ""},
		{"metadata.missing", ""},
		{"", ""},
	} {
		if got := affinityGjsonPath(body, tc.path); got != tc.want {
			t.Fatalf("gjson %q = %q, want %q", tc.path, got, tc.want)
		}
	}
	if got := affinityGjsonPath([]byte(`not-json`), "x"); got != "" {
		t.Fatalf("invalid json must yield empty, got %q", got)
	}
}

func TestRuleAffinityCapacityAndSweep(t *testing.T) {
	resetAffinityForTest(t)
	RuleAffinityStats()
	live := time.Now().Add(time.Hour)
	for i := range maxRuleBindings {
		ruleAffinityCache[strconv.Itoa(i)] = ruleAffinityEntry{expiresAt: live}
	}
	RecordRuleAffinity(AffinityMeta{CacheKey: "extra", TTLSeconds: 60, cacheEpoch: cacheEpoch, ruleRevision: cachedRulesRevision}, 1)
	if len(ruleAffinityCache) != maxRuleBindings {
		t.Fatal("capacity exceeded")
	}
	ruleAffinityCache["0"] = ruleAffinityEntry{}
	if RuleAffinityStats().Total != maxRuleBindings-1 || len(ruleAffinityCache) != maxRuleBindings-1 {
		t.Fatal("expired binding not reclaimed")
	}
}

func TestAffinityStaleCompletion(t *testing.T) {
	resetAffinityForTest(t)
	enableRuleAffinity(t, `[{"name":"r","model_regex":[".*"],"key_sources":[{"type":"gjson","path":"k"}]}]`)
	req := AffinityRequest{TokenID: 7, ModelName: "m", Body: []byte(`{"k":"s"}`)}
	RecordRuleAffinity(ResolveAffinity(req), 1)
	old := ResolveAffinity(req)
	if !RecordRuleAffinity(old, 2) {
		t.Fatal("expected refresh")
	}
	if ClearRuleAffinity(&old) || RecordRuleAffinity(old, 1) || ResolveAffinity(req).ChannelID != 2 {
		t.Fatal("stale completion replaced newer binding")
	}
	current := ResolveAffinity(req)
	ClearAllRuleAffinity()
	if RecordRuleAffinity(current, 2) {
		t.Fatal("cleared binding resurrected")
	}
}

func TestRuleRevisionPreventsStaleWrites(t *testing.T) {
	resetAffinityForTest(t)
	rules := `[{"name":"r","model_regex":[".*"],"key_sources":[{"type":"gjson","path":"k"}]}]`
	enableRuleAffinity(t, rules)
	req := AffinityRequest{ModelName: "m", Body: []byte(`{"k":"s"}`)}
	old := ResolveAffinity(req)
	RecordRuleAffinity(old, 1)
	enableRuleAffinity(t, rules)
	if !ResolveAffinity(req).Found {
		t.Fatal("unchanged rules lost session")
	}
	enableRuleAffinity(t, `[]`)
	enableRuleAffinity(t, rules)
	if RecordRuleAffinity(old, 2) || ResolveAffinity(req).Found {
		t.Fatal("binding crossed configuration generations")
	}
}

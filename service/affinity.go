package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"starhub/setting"
)

// AffinityRequest carries the request fields a rule needs to match and to
// extract its affinity value. The relay flow populates it from its parsed
// request state; it never escapes the request lifetime.
type AffinityRequest struct {
	ModelName   string
	RequestPath string
	UserAgent   string
	Header      http.Header
	Body        []byte
	UsingGroup  string
	TokenID     int64
}

// AffinityMeta is the resolved rule affinity state for one request. Found is
// true only when a rule matched AND the cache holds a channel for it. Matched
// is true when a rule matched (regardless of a cache hit); the caller uses
// Matched to decide rule affinity vs the legacy TokenID-based stickiness.
type AffinityMeta struct {
	RuleName       string
	CacheKey       string
	TTLSeconds     int
	SessionMode    string
	SkipRetry      bool
	KeyHint        string
	KeyFingerprint string
	ChannelID      int64
	Matched        bool
	Found          bool
}

const (
	affinityCacheNamespace = "routing_affinity:v1"
)

// ruleAffinityEntry is one remembered channel plus its expiry. The TTL keeps
// a stale preference from pinning traffic once the bound channel shifts.
type ruleAffinityEntry struct {
	channelID int64
	expiresAt time.Time
}

var (
	ruleAffinityCache    sync.Map // string cacheKey -> ruleAffinityEntry
	affinityRegexCache   sync.Map // pattern string -> *regexp.Regexp
	ruleAffinityClearMu  sync.Mutex
)

// ResolveAffinity evaluates the configured rules in order and returns the
// affinity meta for the first matching rule. When no rule matches, Matched
// and Found are both false and the caller falls back to legacy stickiness.
// When a rule matches but the cache has no entry yet, Matched is true and
// Found is false (ChannelID == 0); the caller records the channel on success.
func ResolveAffinity(req AffinityRequest) AffinityMeta {
	rules := setting.RoutingAffinityRules()
	if len(rules) == 0 || !setting.RoutingAffinityEnabled() {
		return AffinityMeta{}
	}
	for _, rule := range rules {
		if !affinityMatchAny(rule.ModelRegex, req.ModelName) {
			continue
		}
		if len(rule.PathRegex) > 0 && !affinityMatchAny(rule.PathRegex, req.RequestPath) {
			continue
		}
		if len(rule.UserAgentInclude) > 0 && !affinityMatchIncludeFold(rule.UserAgentInclude, req.UserAgent) {
			continue
		}
		value, _ := affinityExtractValue(req, rule.KeySources)
		if value == "" {
			continue
		}
		if rule.ValueRegex != "" && !affinityMatchAny([]string{rule.ValueRegex}, value) {
			continue
		}
		meta := AffinityMeta{
			RuleName:       rule.Name,
			TTLSeconds:     effectiveRuleTTL(rule),
			SessionMode:    setting.EffectiveAffinitySessionMode(rule),
			SkipRetry:      rule.SkipRetryOnFailure,
			KeyHint:        affinityKeyHint(value),
			KeyFingerprint: affinityFingerprint(value),
			Matched:        true,
		}
		if meta.SessionMode == setting.AffinitySessionOff {
			return meta
		}
		meta.CacheKey = affinityCacheKey(rule, req, value)
		if entry, ok := loadRuleAffinity(meta.CacheKey); ok {
			meta.ChannelID = entry.channelID
			meta.Found = true
		}
		return meta
	}
	return AffinityMeta{}
}

// RecordRuleAffinity writes the channel as the preferred one for the meta's
// cache key. Called from noteAffinity on a successful attempt so the next
// request with the same affinity value lands on the same channel.
func RecordRuleAffinity(meta AffinityMeta, channelID int64) {
	if channelID <= 0 || meta.CacheKey == "" || meta.SessionMode == setting.AffinitySessionOff {
		return
	}
	ttl := meta.TTLSeconds
	if ttl <= 0 {
		ttl = int(setting.RoutingAffinityTTL() / time.Second)
	}
	if ttl <= 0 {
		ttl = 300
	}
	if setting.IsCommercialMode() {
		capped := 600
		if ttl > capped {
			ttl = capped
		}
	}
	ruleAffinityCache.Store(meta.CacheKey, ruleAffinityEntry{
		channelID: channelID,
		expiresAt: time.Now().Add(time.Duration(ttl) * time.Second),
	})
}

// ClearRuleAffinity evicts the cache entry for the meta's key. Called when
// the bound channel is unavailable (prefer mode) or fails (upstream error).
func ClearRuleAffinity(meta AffinityMeta) {
	if meta.CacheKey == "" {
		return
	}
	ruleAffinityCache.Delete(meta.CacheKey)
}

// AffinityCacheStats is the admin-visible cache summary.
type AffinityCacheStats struct {
	Enabled    bool              `json:"enabled"`
	Total      int               `json:"total"`
	ByRuleName map[string]int    `json:"by_rule_name"`
	Rules      int               `json:"rules"`
}

// RuleAffinityStats counts live cache entries grouped by rule name. Entries
// whose rule no longer has IncludeRuleName enabled are reported under "".
func RuleAffinityStats() AffinityCacheStats {
	rules := setting.RoutingAffinityRules()
	byRule := make(map[string]int, len(rules))
	for _, r := range rules {
		if r.IncludeRuleName && strings.TrimSpace(r.Name) != "" {
			byRule[r.Name] = 0
		}
	}
	total := 0
	ruleAffinityCache.Range(func(key, value any) bool {
		entry, ok := value.(ruleAffinityEntry)
		if !ok {
			return true
		}
		if time.Now().After(entry.expiresAt) {
			return true // expired; leave for a sweep
		}
		total++
		// The rule segment is the first part of the composite key when
		// IncludeRuleName is on; otherwise the entry cannot be attributed.
		cacheKey, _ := key.(string)
		if name := ruleNameFromKey(cacheKey); name != "" {
			if _, tracked := byRule[name]; tracked {
				byRule[name]++
			} else {
				byRule[""]++
			}
		} else {
			byRule[""]++
		}
		return true
	})
	return AffinityCacheStats{
		Enabled:    setting.RoutingAffinityEnabled() && len(rules) > 0,
		Total:      total,
		ByRuleName: byRule,
		Rules:      len(rules),
	}
}

// ClearAllRuleAffinity evicts every cached entry and returns the count.
func ClearAllRuleAffinity() int {
	count := 0
	ruleAffinityCache.Range(func(key, value any) bool {
		ruleAffinityCache.Delete(key)
		count++
		return true
	})
	return count
}

// ResetRuleAffinityCache clears all rule affinity state (used by tests).
func ResetRuleAffinityCache() {
	ruleAffinityCache = sync.Map{}
	affinityRegexCache = sync.Map{}
}

func effectiveRuleTTL(rule setting.AffinityRule) int {
	if rule.TTLSeconds > 0 {
		return rule.TTLSeconds
	}
	return int(setting.RoutingAffinityTTL() / time.Second)
}

func loadRuleAffinity(key string) (ruleAffinityEntry, bool) {
	raw, ok := ruleAffinityCache.Load(key)
	if !ok {
		return ruleAffinityEntry{}, false
	}
	entry := raw.(ruleAffinityEntry)
	if time.Now().After(entry.expiresAt) {
		ruleAffinityCache.CompareAndDelete(key, entry)
		return ruleAffinityEntry{}, false
	}
	return entry, true
}

func affinityCacheKey(rule setting.AffinityRule, req AffinityRequest, value string) string {
	parts := make([]string, 0, 4)
	if rule.IncludeRuleName && strings.TrimSpace(rule.Name) != "" {
		parts = append(parts, rule.Name)
	}
	if rule.IncludeModelName && req.ModelName != "" {
		parts = append(parts, req.ModelName)
	}
	if rule.IncludeUsingGroup && req.UsingGroup != "" {
		parts = append(parts, req.UsingGroup)
	}
	parts = append(parts, value)
	return affinityCacheNamespace + ":" + strings.Join(parts, ":")
}

// ruleNameFromKey extracts the rule-name segment from a composite cache key.
// Returns "" when the key does not carry a rule segment or the namespace
// prefix is absent.
func ruleNameFromKey(key string) string {
	prefix := affinityCacheNamespace + ":"
	rest, ok := strings.CutPrefix(key, prefix)
	if !ok || rest == "" {
		return ""
	}
	// The first segment up to ":" is the rule name when IncludeRuleName was on.
	if idx := strings.Index(rest, ":"); idx >= 0 {
		return rest[:idx]
	}
	return ""
}

func affinityFingerprint(s string) string {
	if s == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:8]
}

func affinityKeyHint(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	if len(s) <= 12 {
		return s
	}
	return s[:4] + "..." + s[len(s)-4:]
}

func affinityMatchAny(patterns []string, s string) bool {
	if len(patterns) == 0 || s == "" {
		return false
	}
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		re := affinityRegex(pattern)
		if re == nil {
			continue
		}
		if re.MatchString(s) {
			return true
		}
	}
	return false
}

func affinityMatchIncludeFold(patterns []string, s string) bool {
	if len(patterns) == 0 || s == "" {
		return false
	}
	lower := strings.ToLower(s)
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.Contains(lower, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

func affinityRegex(pattern string) *regexp.Regexp {
	if cached, ok := affinityRegexCache.Load(pattern); ok {
		return cached.(*regexp.Regexp)
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		affinityRegexCache.Store(pattern, (*regexp.Regexp)(nil))
		return nil
	}
	affinityRegexCache.Store(pattern, re)
	return re
}

// affinityExtractValue returns the first non-empty value from the key sources
// plus the source that produced it. A gjson source reads a dot-separated path
// from the JSON request body; header/context sources read the named field.
func affinityExtractValue(req AffinityRequest, sources []setting.AffinityKeySource) (string, setting.AffinityKeySource) {
	for _, src := range sources {
		t := strings.TrimSpace(src.Type)
		var value string
		switch t {
		case "request_header":
			if req.Header != nil {
				value = strings.TrimSpace(req.Header.Get(src.Key))
			}
		case "context_string":
			// Context string keys are looked up by the caller's middleware.
			// The relay flow does not carry a generic context bag, so this
			// source is reserved for future wiring; an empty key yields "".
			value = ""
		case "context_int":
			value = ""
		case "gjson":
			value = affinityGjsonPath(req.Body, src.Path)
		default:
			value = ""
		}
		if value != "" {
			return value, src
		}
	}
	return "", setting.AffinityKeySource{}
}

// affinityGjsonPath walks a dot-separated path through a JSON object and
// returns the string form of the leaf value. Numbers and booleans are
// stringified; arrays/objects return their raw JSON. An absent or non-object
// prefix yields "". This is a minimal dependency-free replacement for gjson
// covering the metadata.user_id / prompt_cache_key use cases.
func affinityGjsonPath(body []byte, path string) string {
	path = strings.TrimSpace(path)
	if path == "" || len(body) == 0 {
		return ""
	}
	var current any
	if err := json.Unmarshal(body, &current); err != nil {
		return ""
	}
	for _, segment := range strings.Split(path, ".") {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			return ""
		}
		obj, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		next, present := obj[segment]
		if !present {
			return ""
		}
		current = next
	}
	return affinityValueString(current)
}

func affinityValueString(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		if v {
			return "true"
		}
		return "false"
	case nil:
		return ""
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(raw))
	}
}

// String returns a short human-readable description of the meta for logs.
func (m AffinityMeta) String() string {
	if !m.Matched {
		return "affinity: no rule"
	}
	return fmt.Sprintf("affinity: rule=%s key=%s channel=%d mode=%s", m.RuleName, m.KeyFingerprint, m.ChannelID, m.SessionMode)
}

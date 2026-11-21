package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
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
	ruleRevision                uint64
	cacheEpoch, observedVersion uint64
	KeyIndex                    int
	EndpointGeneration          string
	RuleName                    string
	CacheKey                    string
	TTLSeconds                  int
	SessionMode                 string
	SkipRetry                   bool
	KeyHint                     string
	KeyFingerprint              string
	ChannelID                   int64
	Matched                     bool
	Found                       bool
}

const (
	affinityCacheNamespace = "routing_affinity:v1"
	maxRuleBindings        = 16384
	maxAffinityValueBytes  = 4096
)

// ruleAffinityEntry is one remembered channel plus its expiry. The TTL keeps
// a stale preference from pinning traffic once the bound channel shifts.
type ruleAffinityEntry struct {
	version            uint64
	keyIndex           int
	endpointGeneration string
	ruleName           string
	channelID          int64
	expiresAt          time.Time
}

var (
	cachedRulesRevision     uint64
	cacheEpoch, nextVersion uint64
	ruleAffinityCache       = make(map[string]ruleAffinityEntry)
	nextAffinitySweep       time.Time
	ruleAffinityMu          sync.Mutex
)

// ResolveAffinity evaluates the configured rules in order and returns the
// affinity meta for the first matching rule. When no rule matches, Matched
// and Found are both false and the caller falls back to legacy stickiness.
// When a rule matches but the cache has no entry yet, Matched is true and
// Found is false (ChannelID == 0); the caller records the channel on success.
func ResolveAffinity(req AffinityRequest) AffinityMeta {
	if !setting.RoutingAffinityEnabled() {
		return AffinityMeta{}
	}
	rules, rev := setting.AffinityRulesSnapshot()
	return ResolveAffinityWithRules(req, rules, rev)
}

// ResolveAffinityWithRules matches the current routing hop against a request snapshot.
func ResolveAffinityWithRules(req AffinityRequest, rules []setting.AffinityRule, rev uint64) AffinityMeta {
	// When all rules miss, count only the furthest matching stage for this hop.
	reason := conditionsUnmatched
	for _, rule := range rules {
		if !rule.MatchesModel(req.ModelName) {
			continue
		}
		if !rule.MatchesPath(req.RequestPath) {
			continue
		}
		if len(rule.UserAgentInclude) > 0 && !affinityMatchIncludeFold(rule.UserAgentInclude, req.UserAgent) {
			continue
		}
		value, source := affinityExtractValue(req, rule.KeySources)
		if value == "" {
			reason = max(reason, missingSource)
			continue
		}
		if len(value) > maxAffinityValueBytes || !rule.MatchesValue(value) {
			reason = invalidValue
			continue
		}
		meta := AffinityMeta{
			ruleRevision:   rev,
			RuleName:       rule.Name,
			TTLSeconds:     effectiveRuleTTL(rule),
			SessionMode:    setting.EffectiveAffinitySessionMode(rule),
			SkipRetry:      rule.SkipRetryOnFailure,
			KeyHint:        affinityKeyHint(value),
			KeyFingerprint: affinityFingerprint(value),
			Matched:        true,
		}
		if meta.SessionMode == setting.AffinitySessionOff {
			ruleOff.count()
			return meta
		}
		meta.CacheKey = affinityCacheKey(rule, req, value)
		if strings.TrimSpace(source.Type) == "context_int" {
			// A session named "42" must not share the token-wide binding for token 42.
			meta.CacheKey += ":token"
		}
		ruleAffinityMu.Lock()
		syncRuleRevisionLocked()
		if cachedRulesRevision != rev {
			ruleAffinityMu.Unlock()
			return meta
		}
		meta.cacheEpoch = cacheEpoch
		if entry, ok := loadRuleAffinityLocked(meta.CacheKey); ok {
			meta.observedVersion = entry.version
			meta.ChannelID = entry.channelID
			meta.KeyIndex, meta.EndpointGeneration = entry.keyIndex, entry.endpointGeneration
			meta.Found = true
		} else {
			coldStart.count()
		}
		ruleAffinityMu.Unlock()
		return meta
	}
	if len(rules) > 0 {
		reason.count()
	}
	return AffinityMeta{}
}

// RecordRuleAffinity writes the channel as the preferred one for the meta's
// cache key. Called from noteAffinity on a successful attempt so the next
// request with the same affinity value lands on the same channel.
func RecordRuleAffinity(meta AffinityMeta, channelID int64) bool {
	return RecordRuleAffinityKey(meta, channelID, -1, "")
}

func RecordRuleAffinityKey(meta AffinityMeta, channelID int64, index int, generation string) bool {
	if channelID <= 0 || meta.CacheKey == "" || meta.SessionMode == setting.AffinitySessionOff {
		return false
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
	ruleAffinityMu.Lock()
	defer ruleAffinityMu.Unlock()
	now := time.Now()
	syncRuleRevisionLocked()
	if meta.ruleRevision != cachedRulesRevision {
		writeRevision.count()
		return false
	}
	if cacheEpoch != meta.cacheEpoch {
		writeClear.count()
		return false
	}
	if ruleAffinityCache[meta.CacheKey].version != meta.observedVersion {
		writeRaced.count()
		return false
	}
	if !now.Before(nextAffinitySweep) {
		sweepRuleAffinity(now)
		nextAffinitySweep = now.Add(time.Minute)
	}
	if _, ok := ruleAffinityCache[meta.CacheKey]; !ok && len(ruleAffinityCache) >= maxRuleBindings {
		writeFull.count()
		return false
	}
	nextVersion++
	ruleAffinityCache[meta.CacheKey] = ruleAffinityEntry{version: nextVersion,
		keyIndex: index, endpointGeneration: generation,
		ruleName:  meta.RuleName,
		channelID: channelID,
		expiresAt: time.Now().Add(time.Duration(ttl) * time.Second),
	}
	writeOK.count()
	return true
}

// syncRuleRevisionLocked requires ruleAffinityMu.
func syncRuleRevisionLocked() {
	if rev := setting.AffinityRulesRevision(); rev != cachedRulesRevision {
		clear(ruleAffinityCache)
		cachedRulesRevision = rev
		cacheEpoch++
	}
}

func sweepRuleAffinity(now time.Time) {
	for key, e := range ruleAffinityCache {
		if !now.Before(e.expiresAt) {
			delete(ruleAffinityCache, key)
		}
	}
}

// ClearRuleAffinity evicts the cache entry for the meta's key. Called when
// the bound channel is unavailable (prefer mode) or fails (upstream error).
func ClearRuleAffinity(meta *AffinityMeta) bool {
	if meta.CacheKey == "" || meta.observedVersion == 0 {
		return false
	}
	ruleAffinityMu.Lock()
	defer ruleAffinityMu.Unlock()
	if cacheEpoch != meta.cacheEpoch || ruleAffinityCache[meta.CacheKey].version != meta.observedVersion {
		return false
	}
	delete(ruleAffinityCache, meta.CacheKey)
	meta.observedVersion = 0
	return true
}

// AffinityCacheStats is the admin-visible cache summary.
type AffinityCacheStats struct {
	Instance   string                        `json:"instance"`
	ObservedAt int64                         `json:"observed_at"`
	Events     map[string]uint64             `json:"events"`
	Config     setting.AffinityRuntimeConfig `json:"config"`
	Enabled    bool                          `json:"enabled"`
	Total      int                           `json:"total"`
	ByRuleName map[string]int                `json:"by_rule_name"`
	Rules      int                           `json:"rules"`
}

// RuleAffinityStats counts live cache entries grouped by rule name. Entries
// whose rule no longer has IncludeRuleName enabled are reported under "".
func RuleAffinityStats() AffinityCacheStats {
	rules := setting.RoutingAffinityRules()
	byRule := make(map[string]int, len(rules))
	for _, r := range rules {
		if strings.TrimSpace(r.Name) != "" {
			byRule[r.Name] = 0
		}
	}
	total := 0
	ruleAffinityMu.Lock()
	defer ruleAffinityMu.Unlock()
	sweepRuleAffinity(time.Now())
	syncRuleRevisionLocked()
	for _, entry := range ruleAffinityCache {
		total++
		if name := entry.ruleName; name != "" {
			if _, tracked := byRule[name]; tracked {
				byRule[name]++
			} else {
				byRule[""]++
			}
		} else {
			byRule[""]++
		}
	}
	return AffinityCacheStats{
		Instance: AffinityInstance(), ObservedAt: time.Now().UnixMilli(), Events: affinityEventSnapshot(),
		Config:     setting.CurrentAffinityConfig(),
		Enabled:    setting.RoutingAffinityEnabled() && len(rules) > 0,
		Total:      total,
		ByRuleName: byRule,
		Rules:      len(rules),
	}
}

// ClearAllRuleAffinity evicts every cached entry and returns the count.
func ClearAllRuleAffinity() int {
	ruleAffinityMu.Lock()
	defer ruleAffinityMu.Unlock()
	count := len(ruleAffinityCache)
	cacheEpoch++
	clear(ruleAffinityCache)
	return count
}

// ResetRuleAffinityCache clears all rule affinity state (used by tests).
func ResetRuleAffinityCache() {
	ClearAllRuleAffinity()
}

func effectiveRuleTTL(rule setting.AffinityRule) int {
	if rule.TTLSeconds > 0 {
		return rule.TTLSeconds
	}
	return int(setting.RoutingAffinityTTL() / time.Second)
}

func loadRuleAffinityLocked(key string) (ruleAffinityEntry, bool) {
	entry, ok := ruleAffinityCache[key]
	if !ok {
		return ruleAffinityEntry{}, false
	}
	if time.Now().After(entry.expiresAt) {
		delete(ruleAffinityCache, key)
		return ruleAffinityEntry{}, false
	}
	return entry, true
}

func affinityCacheKey(rule setting.AffinityRule, req AffinityRequest, value string) string {
	// Length-prefix every field; optional legacy flags cannot weaken isolation.
	h := sha256.New()
	for _, field := range []string{strconv.FormatInt(req.TokenID, 10), rule.Name, req.ModelName, req.UsingGroup, value} {
		fmt.Fprintf(h, "%d:", len(field))
		h.Write([]byte(field))
	}
	return affinityCacheNamespace + ":" + hex.EncodeToString(h.Sum(nil))
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

// affinityExtractValue returns the first non-empty value from the key sources
// plus the source that produced it. A gjson source reads a dot-separated path
// from the JSON request body; header/context sources read the named field.
func affinityExtractValue(req AffinityRequest, sources []setting.AffinityKeySource) (string, setting.AffinityKeySource) {
	for _, src := range sources {
		t := strings.TrimSpace(src.Type)
		var value string
		switch t {
		case "context_int":
			if strings.TrimSpace(src.Key) == "token_id" && req.TokenID > 0 {
				value = strconv.FormatInt(req.TokenID, 10)
			}
		case "request_header":
			if req.Header != nil {
				value = strings.TrimSpace(req.Header.Get(strings.TrimSpace(src.Key)))
			}
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
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if !json.Valid(body) || decoder.Decode(&current) != nil {
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
	case json.Number:
		return v.String()
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

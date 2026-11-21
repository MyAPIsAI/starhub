package setting

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Channel affinity (sticky routing) option keys (stored in the option table,
// hot-updatable):
//
//	routing_affinity_enabled          false     master switch
//	routing_affinity_ttl_seconds      300       how long a preference survives
//	routing_affinity_mode             sticky    sticky | hash
//	routing_affinity_cooldown_seconds 30        how long to suppress a failed channel
//	routing_affinity_prefix_cache_enabled  false  enable prompt-prefix scoping
//	routing_affinity_rules            []        rule-driven session affinity (see AffinityRule)
const (
	OptionKeyRoutingAffinityEnabled     = "routing_affinity_enabled"
	OptionKeyRoutingAffinityTTL         = "routing_affinity_ttl_seconds"
	OptionKeyRoutingAffinityMode        = "routing_affinity_mode"
	OptionKeyRoutingAffinityCooldown    = "routing_affinity_cooldown_seconds"
	OptionKeyRoutingAffinityPrefixCache = "routing_affinity_prefix_cache_enabled"
	OptionKeyRoutingAffinityRules       = "routing_affinity_rules"
)

const (
	AffinityModeSticky = "sticky"
	AffinityModeHash   = "hash"

	// Session modes for rule-driven affinity. "inherit" defers to the global
	// default; off disables stickiness for a matching rule; prefer falls back
	// to normal selection when the bound channel is unavailable; strict fails
	// the request instead of falling back.
	AffinitySessionInherit = "inherit"
	AffinitySessionOff     = "off"
	AffinitySessionPrefer  = "prefer"
	AffinitySessionStrict  = "strict"

	DefaultRoutingAffinityTTL    = 5 * time.Minute
	MaxRoutingAffinityTTL        = 24 * time.Hour
	MaxRoutingAffinityTTLSeconds = int64(24 * 60 * 60)

	DefaultRoutingAffinityCooldown    = 30 * time.Second
	MaxRoutingAffinityCooldown        = 10 * time.Minute
	MaxRoutingAffinityCooldownSeconds = int64(10 * 60)

	MaxAffinityRules = 64
)

// AffinityRuntimeConfig contains the global options currently loaded on this replica.
type AffinityRuntimeConfig struct {
	Enabled         bool   `json:"enabled"`
	Mode            string `json:"mode"`
	TTLSeconds      int64  `json:"ttl_seconds"`
	CooldownSeconds int64  `json:"cooldown_seconds"`
	PrefixCache     bool   `json:"prefix_cache"`
}

func CurrentAffinityConfig() AffinityRuntimeConfig {
	routeAffMu.RLock()
	defer routeAffMu.RUnlock()
	return AffinityRuntimeConfig{Enabled: routingAffinityEnabled, Mode: routingAffinityMode, TTLSeconds: int64(routingAffinityTTL / time.Second), CooldownSeconds: int64(routingAffinityCooldown / time.Second), PrefixCache: routingAffinityPrefixCache}
}

// AffinityKeySource extracts the affinity value from one request field. The
// first source in a rule's KeySources that yields a non-empty value wins.
type AffinityKeySource struct {
	Type string `json:"type"` // request_header | gjson | context_int (token_id only)
	Key  string `json:"key,omitempty"`
	Path string `json:"path,omitempty"`
}

// AffinityRule defines one rule-driven sticky binding. Rules are evaluated in
// order; the first matching rule wins. Match conditions are ANDed.
type AffinityRule struct {
	modelPatterns, pathPatterns, valuePatterns []*regexp.Regexp
	Name                                       string              `json:"name"`
	ModelRegex                                 []string            `json:"model_regex"`
	PathRegex                                  []string            `json:"path_regex,omitempty"`
	UserAgentInclude                           []string            `json:"user_agent_include,omitempty"`
	KeySources                                 []AffinityKeySource `json:"key_sources"`
	ValueRegex                                 string              `json:"value_regex,omitempty"`
	TTLSeconds                                 int                 `json:"ttl_seconds,omitempty"`
	SessionMode                                string              `json:"session_mode,omitempty"`
	SkipRetryOnFailure                         bool                `json:"skip_retry_on_failure,omitempty"`
	IncludeRuleName                            bool                `json:"include_rule_name,omitempty"`
	IncludeModelName                           bool                `json:"include_model_name,omitempty"`
	IncludeUsingGroup                          bool                `json:"include_using_group,omitempty"`
}

func (r AffinityRule) MatchesModel(s string) bool { return matchesRulePattern(r.modelPatterns, s) }
func (r AffinityRule) MatchesPath(s string) bool {
	return len(r.pathPatterns) == 0 || matchesRulePattern(r.pathPatterns, s)
}
func (r AffinityRule) MatchesValue(s string) bool {
	return len(r.valuePatterns) == 0 || matchesRulePattern(r.valuePatterns, s)
}
func matchesRulePattern(patterns []*regexp.Regexp, s string) bool {
	for _, p := range patterns {
		if s != "" && p.MatchString(s) {
			return true
		}
	}
	return false
}

func compileRulePatterns(patterns []string) ([]*regexp.Regexp, error) {
	if len(patterns) > 64 {
		return nil, fmt.Errorf("too many patterns")
	}
	out := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" || len(p) > 1024 {
			return nil, fmt.Errorf("invalid pattern length")
		}
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, err
		}
		out = append(out, re)
	}
	return out, nil
}

var (
	routeAffMu                 sync.RWMutex
	routingAffinityEnabled     = false
	routingAffinityTTL         = DefaultRoutingAffinityTTL
	routingAffinityMode        = AffinityModeSticky
	routingAffinityCooldown    = DefaultRoutingAffinityCooldown
	routingAffinityPrefixCache = false
	routingAffinityRules       []AffinityRule
	ruleRevision               uint64
	ruleEncoding               string
)

// AffinityOptionKeys lists the option keys owned by this file.
func AffinityOptionKeys() []string {
	return []string{
		OptionKeyRoutingAffinityEnabled,
		OptionKeyRoutingAffinityTTL,
		OptionKeyRoutingAffinityMode,
		OptionKeyRoutingAffinityCooldown,
		OptionKeyRoutingAffinityPrefixCache,
		OptionKeyRoutingAffinityRules,
	}
}

// IsAffinityOptionKey reports whether the key is a routing-affinity option.
func IsAffinityOptionKey(key string) bool {
	switch key {
	case OptionKeyRoutingAffinityEnabled, OptionKeyRoutingAffinityTTL,
		OptionKeyRoutingAffinityMode, OptionKeyRoutingAffinityCooldown,
		OptionKeyRoutingAffinityPrefixCache, OptionKeyRoutingAffinityRules:
		return true
	}
	return false
}

// RoutingAffinityEnabled reports whether sticky channel routing is turned on.
func RoutingAffinityEnabled() bool {
	routeAffMu.RLock()
	defer routeAffMu.RUnlock()
	return routingAffinityEnabled
}

// RoutingAffinityTTL returns how long a remembered channel stays preferred.
func RoutingAffinityTTL() time.Duration {
	routeAffMu.RLock()
	defer routeAffMu.RUnlock()
	return routingAffinityTTL
}

// RoutingAffinityMode returns the current affinity mode (sticky|hash).
func RoutingAffinityMode() string {
	routeAffMu.RLock()
	defer routeAffMu.RUnlock()
	return routingAffinityMode
}

// RoutingAffinityCooldown returns how long a failed channel stays suppressed.
func RoutingAffinityCooldown() time.Duration {
	routeAffMu.RLock()
	defer routeAffMu.RUnlock()
	return routingAffinityCooldown
}

// RoutingAffinityPrefixCacheEnabled reports whether prompt-prefix cache
// affinity is turned on (makes the key scope include the prefix hash).
func RoutingAffinityPrefixCacheEnabled() bool {
	routeAffMu.RLock()
	defer routeAffMu.RUnlock()
	return routingAffinityPrefixCache
}

// RoutingAffinityRules returns a copy of the rule-driven affinity rules.
// Returns nil when no rules are configured (rule affinity is inactive and
// the legacy TokenID+Model+Group+Prefix affinity governs stickiness).
func RoutingAffinityRules() []AffinityRule {
	rules, _ := AffinityRulesSnapshot()
	return rules
}

func AffinityRulesRevision() uint64 {
	routeAffMu.RLock()
	defer routeAffMu.RUnlock()
	return ruleRevision
}

func AffinityRulesSnapshot() ([]AffinityRule, uint64) {
	routeAffMu.RLock()
	defer routeAffMu.RUnlock()
	return slices.Clone(routingAffinityRules), ruleRevision
}

// EffectiveAffinitySessionMode resolves a rule's session mode against the
// global default. A rule with "inherit" (or empty) takes the global mode;
// the global default is "prefer" when unset.
func EffectiveAffinitySessionMode(rule AffinityRule) string {
	mode := strings.TrimSpace(rule.SessionMode)
	if mode == "" || mode == AffinitySessionInherit {
		return AffinitySessionPrefer
	}
	return mode
}

// validateAffinityRules checks the structural constraints shared by Update
// and Normalize: bounded count, non-empty name, at least one model_regex,
// at least one key_source with a valid type, and a recognized session_mode.
func validateAffinityRules(rules []AffinityRule) error {
	if len(rules) > MaxAffinityRules {
		return fmt.Errorf("too many affinity rules: %d > %d", len(rules), MaxAffinityRules)
	}
	seen := make(map[string]struct{}, len(rules))
	for i, r := range rules {
		name := strings.TrimSpace(r.Name)
		if name == "" {
			return fmt.Errorf("affinity rule[%d]: name is required", i)
		}
		if _, dup := seen[name]; dup {
			return fmt.Errorf("affinity rule[%d]: duplicate name %q", i, name)
		}
		seen[name] = struct{}{}
		if len(r.ModelRegex) == 0 {
			return fmt.Errorf("affinity rule %q: model_regex is required", name)
		}
		var err error
		if rules[i].modelPatterns, err = compileRulePatterns(r.ModelRegex); err != nil {
			return fmt.Errorf("rule %q model_regex: %w", name, err)
		}
		if rules[i].pathPatterns, err = compileRulePatterns(r.PathRegex); err != nil {
			return fmt.Errorf("rule %q path_regex: %w", name, err)
		}
		if r.ValueRegex != "" {
			if rules[i].valuePatterns, err = compileRulePatterns([]string{r.ValueRegex}); err != nil {
				return fmt.Errorf("rule %q value_regex: %w", name, err)
			}
		}
		if len(r.KeySources) == 0 {
			return fmt.Errorf("affinity rule %q: at least one key_source is required", name)
		}
		for j, src := range r.KeySources {
			t := strings.TrimSpace(src.Type)
			if t != "request_header" && t != "gjson" && t != "context_int" {
				return fmt.Errorf("affinity rule %q: key_source[%d]: unknown type %q", name, j, src.Type)
			}
			if t == "context_int" && strings.TrimSpace(src.Key) != "token_id" {
				return fmt.Errorf("affinity rule %q: key_source[%d]: context_int requires key token_id", name, j)
			}
			if t == "request_header" && strings.TrimSpace(src.Key) == "" {
				return fmt.Errorf("affinity rule %q: key_source[%d]: key is required for type %q", name, j, t)
			}
			if t == "gjson" && strings.TrimSpace(src.Path) == "" {
				return fmt.Errorf("affinity rule %q: key_source[%d]: path is required for gjson", name, j)
			}
		}
		mode := strings.TrimSpace(r.SessionMode)
		if mode != "" && mode != AffinitySessionInherit && mode != AffinitySessionOff &&
			mode != AffinitySessionPrefer && mode != AffinitySessionStrict {
			return fmt.Errorf("affinity rule %q: invalid session_mode %q", name, r.SessionMode)
		}
		if r.TTLSeconds < 0 || int64(r.TTLSeconds) > MaxRoutingAffinityTTLSeconds {
			return fmt.Errorf("affinity rule %q: ttl_seconds out of range", name)
		}
	}
	return nil
}

// marshalAffinityRules returns the canonical JSON for the rule list. The
// persisted form is always produced from the parsed value so reloads are
// stable regardless of how the operator formatted the input.
func marshalAffinityRules(rules []AffinityRule) (string, error) {
	canonical, err := json.Marshal(rules)
	if err != nil {
		return "", fmt.Errorf("encode affinity rules: %w", err)
	}
	return string(canonical), nil
}

// UpdateAffinity applies a routing-affinity option key.
func UpdateAffinity(key, value string) error {
	switch key {
	case OptionKeyRoutingAffinityEnabled:
		v, err := parseBoolOption(value)
		if err != nil {
			return fmt.Errorf("invalid value for %s: %w", key, err)
		}
		routeAffMu.Lock()
		routingAffinityEnabled = v
		routeAffMu.Unlock()
		return nil
	case OptionKeyRoutingAffinityTTL:
		var v int64
		if err := json.Unmarshal([]byte(value), &v); err != nil {
			return fmt.Errorf("invalid value for %s: %w", key, err)
		}
		if v <= 0 {
			return fmt.Errorf("invalid value for %s: must be > 0", key)
		}
		ttl := time.Duration(v) * time.Second
		if ttl > MaxRoutingAffinityTTL {
			ttl = MaxRoutingAffinityTTL
		}
		routeAffMu.Lock()
		routingAffinityTTL = ttl
		routeAffMu.Unlock()
		return nil
	case OptionKeyRoutingAffinityMode:
		v := strings.TrimSpace(value)
		if v != AffinityModeSticky && v != AffinityModeHash {
			return fmt.Errorf("invalid value for %s: must be %q or %q", key, AffinityModeSticky, AffinityModeHash)
		}
		routeAffMu.Lock()
		routingAffinityMode = v
		routeAffMu.Unlock()
		return nil
	case OptionKeyRoutingAffinityCooldown:
		var v int64
		if err := json.Unmarshal([]byte(value), &v); err != nil {
			return fmt.Errorf("invalid value for %s: %w", key, err)
		}
		if v <= 0 {
			return fmt.Errorf("invalid value for %s: must be > 0", key)
		}
		ttl := time.Duration(v) * time.Second
		if ttl > MaxRoutingAffinityCooldown {
			ttl = MaxRoutingAffinityCooldown
		}
		routeAffMu.Lock()
		routingAffinityCooldown = ttl
		routeAffMu.Unlock()
		return nil
	case OptionKeyRoutingAffinityPrefixCache:
		v, err := parseBoolOption(value)
		if err != nil {
			return fmt.Errorf("invalid value for %s: %w", key, err)
		}
		routeAffMu.Lock()
		routingAffinityPrefixCache = v
		routeAffMu.Unlock()
		return nil
	case OptionKeyRoutingAffinityRules:
		var rules []AffinityRule
		if err := json.Unmarshal([]byte(value), &rules); err != nil {
			return fmt.Errorf("invalid value for %s: %w", key, err)
		}
		if err := validateAffinityRules(rules); err != nil {
			return fmt.Errorf("invalid value for %s: %w", key, err)
		}
		canonical, err := marshalAffinityRules(rules)
		if err != nil {
			return fmt.Errorf("invalid value for %s: %w", key, err)
		}
		routeAffMu.Lock()
		if canonical != ruleEncoding {
			ruleRevision++
			ruleEncoding = canonical
		}
		routingAffinityRules = rules
		routeAffMu.Unlock()
		return nil
	default:
		return fmt.Errorf("unknown affinity option key: %s", key)
	}
}

// SnapshotAffinity returns the persisted representation of a key.
func SnapshotAffinity(key string) (string, error) {
	routeAffMu.RLock()
	defer routeAffMu.RUnlock()
	switch key {
	case OptionKeyRoutingAffinityEnabled:
		return strconv.FormatBool(routingAffinityEnabled), nil
	case OptionKeyRoutingAffinityTTL:
		return strconv.FormatInt(int64(routingAffinityTTL/time.Second), 10), nil
	case OptionKeyRoutingAffinityMode:
		return routingAffinityMode, nil
	case OptionKeyRoutingAffinityCooldown:
		return strconv.FormatInt(int64(routingAffinityCooldown/time.Second), 10), nil
	case OptionKeyRoutingAffinityPrefixCache:
		return strconv.FormatBool(routingAffinityPrefixCache), nil
	case OptionKeyRoutingAffinityRules:
		canonical, err := marshalAffinityRules(routingAffinityRules)
		if err != nil {
			return "", err
		}
		if canonical == "" {
			canonical = "[]"
		}
		return canonical, nil
	default:
		return "", fmt.Errorf("unknown affinity option key: %s", key)
	}
}

// ResetAffinityToDefaults restores the compiled-in affinity defaults.
func ResetAffinityToDefaults() {
	routeAffMu.Lock()
	defer routeAffMu.Unlock()
	routingAffinityEnabled = false
	routingAffinityTTL = DefaultRoutingAffinityTTL
	routingAffinityMode = AffinityModeSticky
	routingAffinityCooldown = DefaultRoutingAffinityCooldown
	routingAffinityPrefixCache = false
	routingAffinityRules = nil
	ruleRevision++
	ruleEncoding = ""
}

// parseBoolOption accepts a JSON boolean as well as the common textual
// spellings an operator might type into the admin UI (quoted or not).
func parseBoolOption(value string) (bool, error) {
	trimmed := strings.TrimSpace(value)
	var raw any
	if err := json.Unmarshal([]byte(trimmed), &raw); err == nil {
		switch v := raw.(type) {
		case bool:
			return v, nil
		case string:
			trimmed = v
		}
	}
	switch strings.ToLower(trimmed) {
	case "1", "true", "on", "yes":
		return true, nil
	case "0", "false", "off", "no":
		return false, nil
	}
	return false, fmt.Errorf("invalid boolean %q", value)
}

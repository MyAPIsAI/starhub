package setting

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
)

const (
	OptionKeyGroupRatio      = "group_ratio"
	OptionKeyModelRatio      = "model_ratio"
	OptionKeyCompletionRatio = "completion_ratio"
)

const (
	FallbackModelRatio      = 1.0
	FallbackCompletionRatio = 4.0
)

var (
	mu              sync.RWMutex
	groupRatio      = defaultGroupRatio()
	modelRatio      = defaultModelRatio()
	completionRatio = defaultCompletionRatio()
)

func defaultGroupRatio() map[string]float64 {
	return map[string]float64{"default": 1}
}

func defaultModelRatio() map[string]float64 {
	return map[string]float64{
		"gpt-4o":                     2.5,
		"gpt-4o-mini":                0.15,
		"gpt-4":                      10,
		"gpt-4-turbo":                10,
		"gpt-4.1":                    2,
		"gpt-4.1-mini":               0.4,
		"gpt-5":                      1.25,
		"gpt-3.5-turbo":              0.5,
		"deepseek-chat":              0.27,
		"deepseek-reasoner":          0.55,
		"claude-3-5-sonnet-20241022": 3,
		"claude-sonnet-4-6":          3,
		"claude-haiku-4-5":           1,
		"claude-haiku-4-5-20251001":  1,
		"gemini-1.5-flash":           0.075,
		"gemini-1.5-pro":             1.25,
	}
}

func defaultCompletionRatio() map[string]float64 {
	return map[string]float64{
		"gpt-4o":                     4,
		"gpt-4o-mini":                4,
		"gpt-4":                      2,
		"gpt-4-turbo":                2,
		"gpt-4.1":                    4,
		"gpt-4.1-mini":               4,
		"gpt-5":                      8,
		"gpt-3.5-turbo":              3,
		"deepseek-chat":              4,
		"deepseek-reasoner":          2,
		"claude-3-5-sonnet-20241022": 5,
		"claude-sonnet-4-6":          5,
		"claude-haiku-4-5":           5,
		"claude-haiku-4-5-20251001":  5,
		"gemini-1.5-flash":           8,
		"gemini-1.5-pro":             4,
	}
}

func RatioOptionKeys() []string {
	return []string{OptionKeyGroupRatio, OptionKeyModelRatio, OptionKeyCompletionRatio}
}

func IsRatioOptionKey(key string) bool {
	switch key {
	case OptionKeyGroupRatio, OptionKeyModelRatio, OptionKeyCompletionRatio:
		return true
	}
	return false
}

func GroupRatioValue(group string) (float64, bool) {
	mu.RLock()
	defer mu.RUnlock()
	v, ok := groupRatio[group]
	return v, ok
}

func ContainsGroupRatio(group string) bool {
	_, ok := GroupRatioValue(group)
	return ok
}

func ModelRatioValue(model string) float64 {
	v, ok := LookupModelRatio(model)
	if !ok {
		log.Printf("[billing_ratio_fallback] model=%q option=%s fallback=%g", model, OptionKeyModelRatio, v)
	}
	return v
}

// LookupModelRatio returns a ratio and whether it is explicitly priced.
// Callers that choose another pricing mode can defer fallback warnings.
func LookupModelRatio(model string) (float64, bool) {
	mu.RLock()
	defer mu.RUnlock()
	if v, ok := modelRatio[model]; ok {
		return v, true
	}
	return FallbackModelRatio, false
}

func CompletionRatioValue(model string) float64 {
	v, ok := LookupCompletionRatio(model)
	if !ok {
		log.Printf("[billing_ratio_fallback] model=%q option=%s fallback=%g", model, OptionKeyCompletionRatio, v)
	}
	return v
}

// LookupCompletionRatio is the output-ratio equivalent of LookupModelRatio.
func LookupCompletionRatio(model string) (float64, bool) {
	mu.RLock()
	defer mu.RUnlock()
	if v, ok := completionRatio[model]; ok {
		return v, true
	}
	return FallbackCompletionRatio, false
}

func UpdateRatio(key, value string) error {
	ratios, err := parseRatioMap(value)
	if err != nil {
		return fmt.Errorf("invalid value for %s: %w", key, err)
	}
	mu.Lock()
	defer mu.Unlock()
	switch key {
	case OptionKeyGroupRatio:
		groupRatio = ratios
	case OptionKeyModelRatio:
		modelRatio = mergeRatioDefaults(defaultModelRatio(), ratios)
	case OptionKeyCompletionRatio:
		completionRatio = mergeRatioDefaults(defaultCompletionRatio(), ratios)
	default:
		return fmt.Errorf("unknown ratio option key: %s", key)
	}
	return nil
}

// Ratio options override built-in prices. Filling missing entries upgrades
// older configurations without overwriting operator prices, including zero.
func mergeRatioDefaults(defaults, overrides map[string]float64) map[string]float64 {
	for key, value := range overrides {
		defaults[key] = value
	}
	return defaults
}

func SnapshotRatio(key string) (string, error) {
	mu.RLock()
	defer mu.RUnlock()
	var m map[string]float64
	switch key {
	case OptionKeyGroupRatio:
		m = groupRatio
	case OptionKeyModelRatio:
		m = modelRatio
	case OptionKeyCompletionRatio:
		m = completionRatio
	default:
		return "", fmt.Errorf("unknown ratio option key: %s", key)
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func parseRatioMap(value string) (map[string]float64, error) {
	m := map[string]float64{}
	if err := json.Unmarshal([]byte(value), &m); err != nil {
		return nil, err
	}
	for k, v := range m {
		if k == "" {
			return nil, fmt.Errorf("empty ratio key")
		}
		if v < 0 || v != v || v > 1e9 {
			return nil, fmt.Errorf("invalid ratio value for %q: %v", k, v)
		}
	}
	return m, nil
}

// ResetRatiosToDefaults restores the compiled-in default ratios. Tests
// call this to clean up dirty in-memory state from a previous test that
// set custom ratios and failed before it could clean up.
func ResetRatiosToDefaults() {
	mu.Lock()
	defer mu.Unlock()
	groupRatio = defaultGroupRatio()
	modelRatio = defaultModelRatio()
	completionRatio = defaultCompletionRatio()
}

package setting

import (
	"encoding/json"
	"fmt"
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
		"gpt-3.5-turbo":              0.5,
		"deepseek-chat":              0.27,
		"deepseek-reasoner":          0.55,
		"claude-3-5-sonnet-20241022": 3,
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
		"gpt-3.5-turbo":              3,
		"deepseek-chat":              4,
		"deepseek-reasoner":          2,
		"claude-3-5-sonnet-20241022": 5,
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
	mu.RLock()
	defer mu.RUnlock()
	if v, ok := modelRatio[model]; ok {
		return v
	}
	return FallbackModelRatio
}

func CompletionRatioValue(model string) float64 {
	mu.RLock()
	defer mu.RUnlock()
	if v, ok := completionRatio[model]; ok {
		return v
	}
	return FallbackCompletionRatio
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
		modelRatio = ratios
	case OptionKeyCompletionRatio:
		completionRatio = ratios
	default:
		return fmt.Errorf("unknown ratio option key: %s", key)
	}
	return nil
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

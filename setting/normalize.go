package setting

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"starhub/config"
)

// NormalizeOption validates an option and returns its canonical persisted form.
func NormalizeOption(key, value string) (string, error) {
	if config.IsOptionKey(key) {
		return config.NormalizeOption(key, value)
	}
	switch key {
	case OptionKeyGroupRatio, OptionKeyModelRatio, OptionKeyCompletionRatio:
		m, err := parseRatioMap(value)
		if err != nil {
			return "", fmt.Errorf("invalid value for %s: %w", key, err)
		}
		if key == OptionKeyModelRatio {
			m = mergeRatioDefaults(defaultModelRatio(), m)
		} else if key == OptionKeyCompletionRatio {
			m = mergeRatioDefaults(defaultCompletionRatio(), m)
		}
		return marshalStable(m)
	case OptionKeyModelPrice:
		m, err := parseModelPrice(value)
		if err != nil {
			return "", fmt.Errorf("invalid value for %s: %w", key, err)
		}
		return marshalStable(m)
	case OptionKeyModelPriceExpr:
		m, err := parseModelPriceExpr(value)
		if err != nil {
			return "", fmt.Errorf("invalid value for %s: %w", key, err)
		}
		return marshalStable(m)
	case OptionKeyCacheRatio, OptionKeyAudioRatio:
		v, err := parseDetailRatio(value)
		if err != nil {
			return "", fmt.Errorf("invalid value for %s: %w", key, err)
		}
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case OptionKeyGroupGroupRatio:
		m, err := parseGroupGroupRatio(value)
		if err != nil {
			return "", fmt.Errorf("invalid value for %s: %w", key, err)
		}
		return marshalStable(m)
	case OptionKeyAffRebateRatio, OptionKeyAlertErrorRate:
		var v float64
		if err := json.Unmarshal([]byte(value), &v); err != nil || v < 0 || v > 1 {
			return "", fmt.Errorf("invalid value for %s: expected number in [0,1]", key)
		}
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case OptionKeyAffRegisterBonus:
		var v int64
		if err := json.Unmarshal([]byte(value), &v); err != nil || v < 0 {
			return "", fmt.Errorf("invalid value for %s: expected non-negative integer", key)
		}
		return strconv.FormatInt(v, 10), nil
	case OptionKeyAlertWebhookURL:
		var v string
		if err := json.Unmarshal([]byte(value), &v); err != nil {
			v = value
		}
		b, err := json.Marshal(v)
		return string(b), err
	case OptionKeyRoutingAffinityEnabled:
		v, err := parseBoolOption(value)
		if err != nil {
			return "", fmt.Errorf("invalid value for %s: %w", key, err)
		}
		return strconv.FormatBool(v), nil
	case OptionKeyRoutingAffinityTTL:
		var v int64
		if err := json.Unmarshal([]byte(value), &v); err != nil || v <= 0 {
			return "", fmt.Errorf("invalid value for %s: expected positive integer", key)
		}
		if v > MaxRoutingAffinityTTLSeconds {
			v = MaxRoutingAffinityTTLSeconds
		}
		return strconv.FormatInt(v, 10), nil
	case OptionKeyRoutingAffinityMode:
		v := strings.TrimSpace(value)
		if v != AffinityModeSticky && v != AffinityModeHash {
			return "", fmt.Errorf("invalid value for %s: must be %q or %q", key, AffinityModeSticky, AffinityModeHash)
		}
		return v, nil
	case OptionKeyRoutingAffinityCooldown:
		var v int64
		if err := json.Unmarshal([]byte(value), &v); err != nil || v <= 0 {
			return "", fmt.Errorf("invalid value for %s: expected positive integer", key)
		}
		if v > MaxRoutingAffinityCooldownSeconds {
			v = MaxRoutingAffinityCooldownSeconds
		}
		return strconv.FormatInt(v, 10), nil
	case OptionKeyRoutingAffinityPrefixCache:
		v, err := parseBoolOption(value)
		if err != nil {
			return "", fmt.Errorf("invalid value for %s: %w", key, err)
		}
		return strconv.FormatBool(v), nil
	default:
		return "", fmt.Errorf("unknown option key: %s", key)
	}
}

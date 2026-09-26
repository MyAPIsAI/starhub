package setting

import (
	"encoding/json"
	"fmt"
	"strconv"
	"sync"

	"starhub/config"
	"starhub/pkg/billingexpr"
)

// Pricing option keys (stored in the option table, hot-updatable):
//
//	model_price       {"model": usd}    per-call flat price
//	model_price_expr  {"model": "expr"} dynamic/tiered pricing expression
//	cache_ratio       0.5               discount on cached prompt tokens
//	audio_ratio       40                multiplier on audio tokens
const (
	OptionKeyModelPrice     = "model_price"
	OptionKeyModelPriceExpr = "model_price_expr"
	OptionKeyCacheRatio     = "cache_ratio"
	OptionKeyAudioRatio     = "audio_ratio"
)

const (
	FallbackCacheRatio = 0.5
	FallbackAudioRatio = 40.0
)

const (
	maxModelPriceUSD = 10000
	maxDetailRatio   = 1000
)

var (
	optionMu       sync.RWMutex
	priceMu        sync.RWMutex
	modelPrice     = map[string]float64{}
	modelPriceExpr = map[string]string{}
	cacheRatio     = FallbackCacheRatio
	audioRatio     = FallbackAudioRatio
)

// OptionKeys lists every option key the setting package owns, for seeding
// and persistence.
func OptionKeys() []string {
	return append(append(append(append(append(append(RatioOptionKeys(),
		OptionKeyModelPrice, OptionKeyModelPriceExpr,
		OptionKeyCacheRatio, OptionKeyAudioRatio),
		GroupGroupRatioKeys()...),
		AffiliateOptionKeys()...),
		AlertOptionKeys()...),
		AffinityOptionKeys()...), config.OptionKeys()...)
}

// IsOptionKey reports whether the key is a known ratio / pricing option.
func IsOptionKey(key string) bool {
	if config.IsOptionKey(key) {
		return true
	}
	if IsGroupGroupRatioKey(key) || IsAffiliateOptionKey(key) || IsAlertOptionKey(key) || IsAffinityOptionKey(key) {
		return true
	}
	switch key {
	case OptionKeyModelPrice, OptionKeyModelPriceExpr,
		OptionKeyCacheRatio, OptionKeyAudioRatio:
		return true
	}
	return IsRatioOptionKey(key)
}

// UpdateOption validates and applies any known option key.
func UpdateOption(key, value string) error {
	optionMu.Lock()
	defer optionMu.Unlock()
	return updateOption(key, value)
}

// ApplyOptions replaces a validated option set without exposing partial prices.
func ApplyOptions(values map[string]string) error {
	for key, value := range values {
		if _, err := NormalizeOption(key, value); err != nil {
			return err
		}
	}
	optionMu.Lock()
	defer optionMu.Unlock()
	for key, value := range values {
		if err := updateOption(key, value); err != nil {
			return err
		}
	}
	return nil
}

func updateOption(key, value string) error {
	if config.IsOptionKey(key) {
		return config.ApplyOption(key, value)
	}
	if IsGroupGroupRatioKey(key) {
		return UpdateGroupGroupRatio(value)
	}
	if IsAffiliateOptionKey(key) {
		return UpdateAffiliate(key, value)
	}
	if IsAlertOptionKey(key) {
		return UpdateAlert(key, value)
	}
	if IsAffinityOptionKey(key) {
		return UpdateAffinity(key, value)
	}
	if IsRatioOptionKey(key) {
		return UpdateRatio(key, value)
	}
	return UpdatePricing(key, value)
}

// SnapshotOption returns the current persisted representation of a key.
func SnapshotOption(key string) (string, error) {
	optionMu.RLock()
	defer optionMu.RUnlock()
	return snapshotOption(key)
}

func snapshotOption(key string) (string, error) {
	if config.IsOptionKey(key) {
		return config.SnapshotOption(key)
	}
	if IsGroupGroupRatioKey(key) {
		return SnapshotGroupGroupRatio()
	}
	if IsAffiliateOptionKey(key) {
		return SnapshotAffiliate(key)
	}
	if IsAlertOptionKey(key) {
		return SnapshotAlert(key)
	}
	if IsAffinityOptionKey(key) {
		return SnapshotAffinity(key)
	}
	if IsRatioOptionKey(key) {
		return SnapshotRatio(key)
	}
	return SnapshotPricing(key)
}

// UpdatePricing validates and applies one pricing option key.
func UpdatePricing(key, value string) error {
	switch key {
	case OptionKeyModelPrice:
		prices, err := parseModelPrice(value)
		if err != nil {
			return fmt.Errorf("invalid value for %s: %w", key, err)
		}
		priceMu.Lock()
		modelPrice = prices
		priceMu.Unlock()
		return nil
	case OptionKeyModelPriceExpr:
		exprs, err := parseModelPriceExpr(value)
		if err != nil {
			return fmt.Errorf("invalid value for %s: %w", key, err)
		}
		priceMu.Lock()
		modelPriceExpr = exprs
		priceMu.Unlock()
		return nil
	case OptionKeyCacheRatio:
		v, err := parseDetailRatio(value)
		if err != nil {
			return fmt.Errorf("invalid value for %s: %w", key, err)
		}
		priceMu.Lock()
		cacheRatio = v
		priceMu.Unlock()
		return nil
	case OptionKeyAudioRatio:
		v, err := parseDetailRatio(value)
		if err != nil {
			return fmt.Errorf("invalid value for %s: %w", key, err)
		}
		priceMu.Lock()
		audioRatio = v
		priceMu.Unlock()
		return nil
	default:
		return fmt.Errorf("unknown pricing option key: %s", key)
	}
}

// SnapshotPricing returns the persisted representation of a pricing key.
func SnapshotPricing(key string) (string, error) {
	priceMu.RLock()
	defer priceMu.RUnlock()
	switch key {
	case OptionKeyModelPrice:
		return marshalStable(modelPrice)
	case OptionKeyModelPriceExpr:
		return marshalStable(modelPriceExpr)
	case OptionKeyCacheRatio:
		return strconv.FormatFloat(cacheRatio, 'f', -1, 64), nil
	case OptionKeyAudioRatio:
		return strconv.FormatFloat(audioRatio, 'f', -1, 64), nil
	default:
		return "", fmt.Errorf("unknown pricing option key: %s", key)
	}
}

// ModelPriceValue returns the per-call USD price, if configured.
func ModelPriceValue(model string) (float64, bool) {
	priceMu.RLock()
	defer priceMu.RUnlock()
	v, ok := modelPrice[model]
	return v, ok
}

// ModelPriceExpr returns the dynamic pricing expression, if configured.
func ModelPriceExpr(model string) (string, bool) {
	priceMu.RLock()
	defer priceMu.RUnlock()
	v, ok := modelPriceExpr[model]
	return v, ok
}

// CacheRatioValue is the discount multiplier for cached prompt tokens.
func CacheRatioValue() float64 {
	priceMu.RLock()
	defer priceMu.RUnlock()
	return cacheRatio
}

// AudioRatioValue is the premium multiplier for audio tokens.
func AudioRatioValue() float64 {
	priceMu.RLock()
	defer priceMu.RUnlock()
	return audioRatio
}

func parseModelPrice(value string) (map[string]float64, error) {
	m := map[string]float64{}
	if err := json.Unmarshal([]byte(value), &m); err != nil {
		return nil, err
	}
	for k, v := range m {
		if k == "" {
			return nil, fmt.Errorf("empty price key")
		}
		if v < 0 || v != v || v > maxModelPriceUSD {
			return nil, fmt.Errorf("invalid price for %q: %v", k, v)
		}
	}
	return m, nil
}

func parseModelPriceExpr(value string) (map[string]string, error) {
	m := map[string]string{}
	if err := json.Unmarshal([]byte(value), &m); err != nil {
		return nil, err
	}
	for k, expr := range m {
		if k == "" {
			return nil, fmt.Errorf("empty expression key")
		}
		if err := billingexpr.Validate(expr); err != nil {
			return nil, fmt.Errorf("invalid expression for %q: %w", k, err)
		}
	}
	return m, nil
}

func parseDetailRatio(value string) (float64, error) {
	var v float64
	if err := json.Unmarshal([]byte(value), &v); err != nil {
		return 0, err
	}
	if v < 0 || v != v || v > maxDetailRatio {
		return 0, fmt.Errorf("invalid ratio %v", v)
	}
	return v, nil
}

// marshalStable marshals a map (encoding/json sorts keys, so snapshots
// round-trip stably).
func marshalStable[V any](m map[string]V) (string, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ResetPricingToDefaults restores the compiled-in default pricing. Tests
// call this to clean up dirty in-memory state from a previous test.
func ResetPricingToDefaults() {
	priceMu.Lock()
	defer priceMu.Unlock()
	modelPrice = map[string]float64{}
	modelPriceExpr = map[string]string{}
	cacheRatio = FallbackCacheRatio
	audioRatio = FallbackAudioRatio
}

// ResetAllToDefaults restores every setting to its compiled-in defaults.
// Tests call this before a fresh DB to prevent dirty state from a previous
// test (that may have failed before cleanup) from being seeded into the
// new DB.
func ResetAllToDefaults() {
	ResetRatiosToDefaults()
	ResetPricingToDefaults()
	ResetGroupGroupRatioToDefaults()
	ResetAffiliateToDefaults()
	ResetAlertToDefaults()
	ResetAffinityToDefaults()
}

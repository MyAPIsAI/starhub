package setting

import (
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"starhub/pkg/pricingtime"
)

const OptionKeyModelTimeTokenPrices = "model_time_token_prices"

var tokenTimePricing atomic.Pointer[pricingtime.TokenCatalog]
var ErrTokenPriceMode = errors.New("independent token windows require token billing; remove the windows before changing billing mode")

func TokenTimePricingAt(model string, at time.Time) (pricingtime.TokenAdjustment, bool) {
	return tokenTimePricing.Load().Match(model, at)
}

func snapshotTokenTimePricing() string {
	if catalog := tokenTimePricing.Load(); catalog != nil {
		return catalog.JSON()
	}
	return "{}"
}

// ValidateTokenPriceModes checks a complete persisted three-option contract.
// Multiplier schedules stack with these base rates; flat/expression modes may
// not silently hide configured absolute token windows.
func ValidateTokenPriceModes(values map[string]string) error {
	catalog, err := pricingtime.DecodeTokenPrices(values[OptionKeyModelTimeTokenPrices])
	if err != nil {
		return err
	}
	prices, err := parseModelPrice(values[OptionKeyModelPrice])
	if err != nil {
		return err
	}
	expressions, err := parseModelPriceExpr(values[OptionKeyModelPriceExpr])
	if err != nil {
		return err
	}
	for _, model := range catalog.PricedModels() {
		if prices[model] > 0 || expressions[model] != "" {
			return fmt.Errorf("%w: %s", ErrTokenPriceMode, model)
		}
	}
	return nil
}

// Called under optionMu before a complete or partial settings replacement.
func validateTokenPriceUpdate(updates map[string]string) error {
	_, tokenChanged := updates[OptionKeyModelTimeTokenPrices]
	_, flatChanged := updates[OptionKeyModelPrice]
	_, expressionChanged := updates[OptionKeyModelPriceExpr]
	if !tokenChanged && !flatChanged && !expressionChanged {
		return nil
	}
	values := map[string]string{OptionKeyModelTimeTokenPrices: snapshotTokenTimePricing()}
	for _, key := range []string{OptionKeyModelPrice, OptionKeyModelPriceExpr} {
		value, err := SnapshotPricing(key)
		if err != nil {
			return err
		}
		values[key] = value
	}
	for key := range values {
		if value, ok := updates[key]; ok {
			values[key] = value
		}
	}
	return ValidateTokenPriceModes(values)
}

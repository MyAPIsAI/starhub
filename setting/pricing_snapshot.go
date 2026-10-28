package setting

import (
	"starhub/pkg/pricingtime"
	"time"
)

type PricingInputs struct {
	ModelRatio      float64
	HasModelRatio   bool
	CompletionRatio float64
	HasCompletion   bool
	GroupRatio      float64
	GroupGroupRatio float64
	CacheRatio      float64
	AudioRatio      float64
	ModelPriceUSD   float64
	HasModelPrice   bool
	PriceExpr       string
	TimeAdjustment  pricingtime.Adjustment
	PricingAt       time.Time
}

// ResolvePricing reads every billing option under one update barrier.
func ResolvePricing(model, group, userGroup string) PricingInputs {
	optionMu.RLock()
	defer optionMu.RUnlock()
	return resolvePricing(model, group, userGroup, time.Now())
}

func ResolvePricingAt(model, group, userGroup string, at time.Time) PricingInputs {
	optionMu.RLock()
	defer optionMu.RUnlock()
	return resolvePricing(model, group, userGroup, at)
}

// ResolvePricingModels gives a catalog response one configuration and instant.
func ResolvePricingModels(models []string, group, userGroup string) map[string]PricingInputs {
	optionMu.RLock()
	defer optionMu.RUnlock()
	at := time.Now()
	result := make(map[string]PricingInputs, len(models))
	for _, model := range models {
		result[model] = resolvePricing(model, group, userGroup, at)
	}
	return result
}

// ResolvePricingGroups captures every eligible billing group under one option
// barrier, so a later retry cannot mix versions of the same request's prices.
func ResolvePricingGroups(model string, groups []string, userGroup string) map[string]PricingInputs {
	optionMu.RLock()
	defer optionMu.RUnlock()
	return resolvePricingGroups(model, groups, userGroup, time.Now())
}

func ResolvePricingGroupsAt(model string, groups []string, userGroup string, at time.Time) map[string]PricingInputs {
	optionMu.RLock()
	defer optionMu.RUnlock()
	return resolvePricingGroups(model, groups, userGroup, at)
}

func resolvePricingGroups(model string, groups []string, userGroup string, at time.Time) map[string]PricingInputs {
	result := make(map[string]PricingInputs, len(groups))
	for _, group := range groups {
		result[group] = resolvePricing(model, group, userGroup, at)
	}
	return result
}

func resolvePricing(model, group, userGroup string, at time.Time) PricingInputs {
	p := PricingInputs{}
	p.ModelRatio, p.HasModelRatio = LookupModelRatio(model)
	p.CompletionRatio, p.HasCompletion = LookupCompletionRatio(model)
	p.GroupRatio, _ = GroupRatioValue(group)
	if p.GroupRatio <= 0 {
		p.GroupRatio = 1
	}
	p.GroupGroupRatio = 1
	if userGroup != "" && group != "" {
		p.GroupGroupRatio = GroupGroupRatioValue(userGroup, group)
	}
	p.CacheRatio = CacheRatioValue()
	p.AudioRatio = AudioRatioValue()
	p.ModelPriceUSD, p.HasModelPrice = ModelPriceValue(model)
	p.PriceExpr, _ = ModelPriceExpr(model)
	p.TimeAdjustment = TimePricingAt(model, at)
	p.PricingAt = at.UTC()
	return p
}

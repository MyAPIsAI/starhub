package setting

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
	PriceExpr       string
}

// ResolvePricing reads every billing option under one update barrier.
func ResolvePricing(model, group, userGroup string) PricingInputs {
	optionMu.RLock()
	defer optionMu.RUnlock()
	return resolvePricing(model, group, userGroup)
}

// ResolvePricingGroups captures every eligible billing group under one option
// barrier, so a later retry cannot mix versions of the same request's prices.
func ResolvePricingGroups(model string, groups []string, userGroup string) map[string]PricingInputs {
	optionMu.RLock()
	defer optionMu.RUnlock()
	result := make(map[string]PricingInputs, len(groups))
	for _, group := range groups {
		result[group] = resolvePricing(model, group, userGroup)
	}
	return result
}

func resolvePricing(model, group, userGroup string) PricingInputs {
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
	p.ModelPriceUSD, _ = ModelPriceValue(model)
	p.PriceExpr, _ = ModelPriceExpr(model)
	return p
}

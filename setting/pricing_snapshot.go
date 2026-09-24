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

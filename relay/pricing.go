package relay

import (
	"log"

	"starhub/common"
	"starhub/pkg/billingexpr"
	"starhub/setting"
)

// PriceData carries every multiplier of one request's bill:
//
//	base price   = per-token ratio billing, per-call fixed price, or a
//	               billing expression (highest priority)
//	final quota  = base x group_ratio x other_ratio
//
// other_ratio accumulates the special multipliers (audio token share, image
// resolution, video seconds, ...) via AddOtherRatio — the image / video
// relays (P3) plug in here without touching the settlement core.
type PriceData struct {
	ModelName       string
	ModelRatio      float64
	CompletionRatio float64
	// ModelPriceUSD > 0 switches the model to per-call billing.
	ModelPriceUSD float64
	// PriceExpr, when configured, wins over per-call and token billing.
	PriceExpr string

	GroupRatio float64
	CacheRatio float64
	AudioRatio float64
	OtherRatio float64
	// UserGroup is the user's own group (for the group x group matrix).
	// Empty means no extra matrix multiplier.
	UserGroup string
	// UsingGroup is the group the request is billed through (the token /
	// channel group). Empty means no extra matrix multiplier.
	UsingGroup string
}

// NewPriceData resolves all billing inputs for a model / group from the
// current settings. The user group (for the group x group matrix) defaults
// to empty; use NewPriceDataWithUserGroup when the matrix should apply.
func NewPriceData(modelName, group string) *PriceData {
	groupRatio, ok := setting.GroupRatioValue(group)
	if !ok || groupRatio <= 0 {
		groupRatio = 1
	}
	pd := &PriceData{
		ModelName:       modelName,
		ModelRatio:      setting.ModelRatioValue(modelName),
		CompletionRatio: setting.CompletionRatioValue(modelName),
		GroupRatio:      groupRatio,
		CacheRatio:      setting.CacheRatioValue(),
		AudioRatio:      setting.AudioRatioValue(),
		OtherRatio:      1,
		UsingGroup:      group,
	}
	if price, ok := setting.ModelPriceValue(modelName); ok && price > 0 {
		pd.ModelPriceUSD = price
	}
	if expr, ok := setting.ModelPriceExpr(modelName); ok && expr != "" {
		pd.PriceExpr = expr
	}
	return pd
}

// NewPriceDataWithUserGroup is NewPriceData plus the user's own group, so the
// group x group matrix multiplier can apply. The matrix entry for
// (userGroup, usingGroup) multiplies the effective ratio; a missing entry is
// 1.0 (no extra multiplier).
func NewPriceDataWithUserGroup(modelName, group, userGroup string) *PriceData {
	pd := NewPriceData(modelName, group)
	pd.UserGroup = userGroup
	return pd
}

// AddOtherRatio multiplies a special billing multiplier (audio / image
// resolution / video seconds ...) into the chain. The product is clamped at
// common.MaxRatioProduct — a clamped value is audited with the
// quota_saturation marker instead of overflowing downstream.
func (p *PriceData) AddOtherRatio(ratio float64) {
	p.OtherRatio = common.ClampRatio(p.OtherRatio*ratio, "other")
}

// EffectiveRatio is the full multiplier applied on top of the base price.
// The group x group matrix multiplier applies when both UserGroup and
// UsingGroup are set; a missing matrix entry is 1.0 (no extra multiplier).
func (p *PriceData) EffectiveRatio() float64 {
	r := p.GroupRatio * p.OtherRatio
	if p.UserGroup != "" && p.UsingGroup != "" {
		r *= setting.GroupGroupRatioValue(p.UserGroup, p.UsingGroup)
	}
	return common.ClampRatio(r, "effective")
}

// IsExpression reports whether the model is billed by pricing expression.
func (p *PriceData) IsExpression() bool { return p.PriceExpr != "" }

// IsPerCall reports whether the model is billed per call (fixed price).
func (p *PriceData) IsPerCall() bool { return p.ModelPriceUSD > 0 }

// ComputeUsageQuota prices one usage report:
//
//	expression     -> Eval(expr, usage vars) USD
//	per-call price -> ModelPriceUSD
//	otherwise      -> token billing with cache / audio detail
//
// Every float -> int conversion goes through common.QuotaFromFloat64
// (saturating, audited); a failed expression evaluation falls back to token
// billing and is audited with the quota_saturation marker.
func ComputeUsageQuota(p *PriceData, usage *Usage) int64 {
	if usage == nil {
		usage = &Usage{}
	}
	if p.IsExpression() {
		vars := map[string]float64{
			"prompt_tokens":     float64(usage.PromptTokens),
			"completion_tokens": float64(usage.CompletionTokens),
			"total_tokens":      float64(usage.PromptTokens + usage.CompletionTokens),
		}
		usd, err := billingexpr.Eval(p.PriceExpr, vars)
		if err != nil {
			log.Printf("[%s] model %s expression %q failed (%v), falling back to token billing",
				common.QuotaSaturation, p.ModelName, p.PriceExpr, err)
		} else {
			return priceUSDToQuota(p, usd)
		}
	}
	if p.IsPerCall() {
		return priceUSDToQuota(p, p.ModelPriceUSD)
	}
	return tokenUsageQuota(p, usage)
}

// priceUSDToQuota converts a per-call / expression USD price into quota.
// A configured positive price always bills at least 1 quota so rounding can
// never silently wash out a paid model.
func priceUSDToQuota(p *PriceData, usd float64) int64 {
	if usd <= 0 {
		return 0
	}
	quota := common.QuotaFromFloat64(usd * float64(common.QuotaPerUSD) * p.EffectiveRatio())
	if quota == 0 {
		quota = 1
	}
	return quota
}

// tokenUsageQuota implements the per-token formula:
//
//	effective input  = text + cached x cache_ratio + audio x audio_ratio
//	effective output = text x completion_ratio + audio x audio_ratio
//	quota            = (in + out) x model_ratio / 1e6 x QuotaPerUSD x ratios
//
// Cached / audio tokens are subsets of the usage totals, so a lying upstream
// cannot inflate the discount beyond the totals (clamped in Usage helpers).
func tokenUsageQuota(p *PriceData, usage *Usage) int64 {
	promptTokens := max64(usage.PromptTokens, 0)
	completionTokens := max64(usage.CompletionTokens, 0)

	cached := usage.CachedTokens()
	inputAudio := usage.InputAudioTokens()
	textInput := promptTokens - cached - inputAudio
	if textInput < 0 {
		textInput = 0
	}

	outputAudio := usage.OutputAudioTokens()
	textOutput := completionTokens - outputAudio
	if textOutput < 0 {
		textOutput = 0
	}

	effectiveInput := float64(textInput) +
		float64(cached)*p.CacheRatio +
		float64(inputAudio)*p.AudioRatio
	effectiveOutput := float64(textOutput)*p.CompletionRatio +
		float64(outputAudio)*p.AudioRatio

	cost := (effectiveInput + effectiveOutput) * p.ModelRatio / 1e6 *
		float64(common.QuotaPerUSD) * p.EffectiveRatio()
	quota := common.QuotaFromFloat64(cost)
	if quota == 0 && promptTokens+completionTokens > 0 {
		// Any real token usage bills at least 1 quota.
		quota = 1
	}
	return quota
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

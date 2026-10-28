package relay

import (
	"log"
	"time"

	"starhub/common"
	"starhub/pkg/billingexpr"
	"starhub/pkg/pricingtime"
	"starhub/setting"
)

// PriceData carries every multiplier of one request's bill:
//
//	base price   = per-token ratio billing, per-call fixed price, or a
//	               billing expression (highest priority)
//	final quota  = base x group_ratio x time_ratio x other_ratio
//
// other_ratio accumulates the special multipliers (audio token share, image
// resolution, video seconds, ...) via AddOtherRatio — the image / video
// relays (P3) plug in here without touching the settlement core.
type PriceData struct {
	ModelName              string
	ModelRatio             float64
	CompletionRatio        float64
	missingModelRatio      bool
	missingCompletionRatio bool
	// ModelPriceUSD > 0 switches the model to per-call billing.
	ModelPriceUSD  float64
	HasModelPrice  bool
	TimeAdjustment pricingtime.Adjustment
	PricingTime    time.Time
	// PriceExpr, when configured, wins over per-call and token billing.
	PriceExpr string

	GroupRatio      float64
	GroupGroupRatio float64
	CacheRatio      float64
	AudioRatio      float64
	OtherRatio      float64
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
	return newPriceData(modelName, group, "")
}

func newPriceData(modelName, group, userGroup string) *PriceData {
	inputs := setting.ResolvePricing(modelName, group, userGroup)
	return priceDataFromInputs(modelName, group, userGroup, inputs)
}

// CapturePricing returns value snapshots for a request's eligible groups.
// Callers copy a value before applying per-attempt special multipliers.
func CapturePricing(modelName string, groups []string, userGroup string) map[string]PriceData {
	inputs := setting.ResolvePricingGroups(modelName, groups, userGroup)
	return capturedPriceData(modelName, userGroup, inputs)
}

func CapturePricingAt(modelName string, groups []string, userGroup string, at time.Time) map[string]PriceData {
	return capturedPriceData(modelName, userGroup, setting.ResolvePricingGroupsAt(modelName, groups, userGroup, at))
}

func NewPriceDataAt(modelName, group, userGroup string, at time.Time) *PriceData {
	return priceDataFromInputs(modelName, group, userGroup, setting.ResolvePricingAt(modelName, group, userGroup, at))
}

func capturedPriceData(modelName, userGroup string, inputs map[string]setting.PricingInputs) map[string]PriceData {
	result := make(map[string]PriceData, len(inputs))
	for group, value := range inputs {
		result[group] = *priceDataFromInputs(modelName, group, userGroup, value)
	}
	return result
}

func priceDataFromInputs(modelName, group, userGroup string, inputs setting.PricingInputs) *PriceData {
	pd := &PriceData{
		ModelName:              modelName,
		ModelRatio:             inputs.ModelRatio,
		CompletionRatio:        inputs.CompletionRatio,
		missingModelRatio:      !inputs.HasModelRatio,
		missingCompletionRatio: !inputs.HasCompletion,
		GroupRatio:             inputs.GroupRatio,
		GroupGroupRatio:        inputs.GroupGroupRatio,
		CacheRatio:             inputs.CacheRatio,
		AudioRatio:             inputs.AudioRatio,
		OtherRatio:             1,
		HasModelPrice:          inputs.HasModelPrice,
		TimeAdjustment:         inputs.TimeAdjustment,
		PricingTime:            inputs.PricingAt,
		UsingGroup:             group,
		UserGroup:              userGroup,
	}
	if inputs.ModelPriceUSD > 0 {
		pd.ModelPriceUSD = inputs.ModelPriceUSD
	}
	if inputs.PriceExpr != "" {
		pd.PriceExpr = inputs.PriceExpr
	}
	return pd
}

// NewPriceDataWithUserGroup is NewPriceData plus the user's own group, so the
// group x group matrix multiplier can apply. The matrix entry for
// (userGroup, usingGroup) multiplies the effective ratio; a missing entry is
// 1.0 (no extra multiplier).
func NewPriceDataWithUserGroup(modelName, group, userGroup string) *PriceData {
	return newPriceData(modelName, group, userGroup)
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
	// Old persisted snapshots have no time adjustment; positive-only schedule
	// validation makes zero an unambiguous legacy default of one.
	if p.TimeAdjustment.Multiplier != 0 {
		r *= p.TimeAdjustment.Multiplier
	}
	if p.UserGroup != "" && p.UsingGroup != "" {
		r *= p.GroupGroupRatio
	}
	return common.ClampRatio(r, "effective")
}

// IsExpression reports whether the model is billed by pricing expression.
func (p *PriceData) IsExpression() bool { return p.PriceExpr != "" }

// IsPerCall reports whether the model is billed per call (fixed price).
func (p *PriceData) IsPerCall() bool { return p.ModelPriceUSD > 0 }

// evalExpression evaluates the pricing expression against vars and converts
// the USD result to quota. On failure it audits with the quota_saturation
// marker and returns (0, false) so the caller can fall back to per-call or
// token billing. This is the single expression-evaluation entry point used
// by ComputeUsageQuota, estimatePreConsume, and the image / video billing
// paths — keeping the variable set consistent and the audit log uniform.
func evalExpression(p *PriceData, vars map[string]float64) (int64, bool) {
	usd, err := billingexpr.Eval(p.PriceExpr, vars)
	if err != nil {
		log.Printf("[%s] model %s expression %q failed (%v), falling back",
			common.QuotaSaturation, p.ModelName, p.PriceExpr, err)
		return 0, false
	}
	return priceUSDToQuota(p, usd), true
}

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
		if q, ok := evalExpression(p, vars); ok {
			return q
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
	if p.missingModelRatio {
		log.Printf("[billing_ratio_fallback] model=%q option=%s fallback=%g", p.ModelName, setting.OptionKeyModelRatio, p.ModelRatio)
	}
	if p.missingCompletionRatio {
		log.Printf("[billing_ratio_fallback] model=%q option=%s fallback=%g", p.ModelName, setting.OptionKeyCompletionRatio, p.CompletionRatio)
	}
	promptTokens := max64(usage.PromptTokens, 0)
	completionTokens := max64(usage.CompletionTokens, 0)

	cached := usage.CachedTokens()
	inputAudio := usage.InputAudioTokens()
	// Cache writes are included once at the normal platform input rate.
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

package relay

import (
	"starhub/common"
	"starhub/relay/channel"
)

type TaskBillingSnapshot struct {
	PriceData PriceData           `json:"price_data"`
	Estimate  channel.TaskPricing `json:"estimate"`
}

// TaskPricingQuota converts a platform's TaskPricing into quota (the
// billing half of the P3.3 lifecycle: platforms report prices, the relay
// owns the quota math).
//
//	per-call platforms:  PerCallUSD x Units x group_ratio
//	token platforms:     token billing on Prompt/CompletionTokens
func TaskPricingQuota(modelName, group string, p channel.TaskPricing) int64 {
	return TaskPricingQuotaWithUserGroup(modelName, group, "", p)
}

// TaskPricingQuotaWithUserGroup is TaskPricingQuota with the user's own
// group for the group x group matrix multiplier.
func TaskPricingQuotaWithUserGroup(modelName, group, userGroup string, p channel.TaskPricing) int64 {
	return TaskPricingQuotaWithPriceData(NewPriceDataWithUserGroup(modelName, group, userGroup), p)
}

// TaskPricingQuotaWithPriceData uses the pricing snapshot captured at submit.
func TaskPricingQuotaWithPriceData(pd *PriceData, p channel.TaskPricing) int64 {
	if p.PerCallUSD > 0 && p.Units > 0 {
		return common.QuotaFromFloat64(
			p.PerCallUSD * p.Units * float64(common.QuotaPerUSD) * pd.EffectiveRatio())
	}
	if p.PerCallUSD > 0 && p.Units <= 0 {
		return 0 // zero units on a priced platform = full refund / free
	}
	return tokenUsageQuota(pd, &Usage{
		PromptTokens:     p.PromptTokens,
		CompletionTokens: p.CompletionTokens,
	})
}

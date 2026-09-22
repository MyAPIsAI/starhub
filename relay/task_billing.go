package relay

import (
	"starhub/common"
	"starhub/relay/channel"
	"starhub/setting"
)

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
	if p.PerCallUSD > 0 && p.Units > 0 {
		groupRatio, ok := setting.GroupRatioValue(group)
		if !ok || groupRatio <= 0 {
			groupRatio = 1
		}
		matrix := 1.0
		if userGroup != "" {
			matrix = setting.GroupGroupRatioValue(userGroup, group)
		}
		return common.QuotaFromFloat64(
			p.PerCallUSD * p.Units * float64(common.QuotaPerUSD) * groupRatio * matrix)
	}
	if p.PerCallUSD > 0 && p.Units <= 0 {
		return 0 // zero units on a priced platform = full refund / free
	}
	pd := NewPriceDataWithUserGroup(modelName, group, userGroup)
	return tokenUsageQuota(pd, &Usage{
		PromptTokens:     p.PromptTokens,
		CompletionTokens: p.CompletionTokens,
	})
}

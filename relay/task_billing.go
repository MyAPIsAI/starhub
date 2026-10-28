package relay

import (
	"encoding/json"
	"fmt"

	"starhub/common"
	"starhub/relay/channel"
)

type TaskBillingSnapshot struct {
	PriceData PriceData           `json:"price_data"`
	Estimate  channel.TaskPricing `json:"estimate"`
}

// Absolute token tariffs are not enabled for asynchronous tasks until every
// persisted-snapshot consumer supports them. Reject before submitting upstream;
// older workers would otherwise silently ignore the new PriceData field.
func (s TaskBillingSnapshot) MarshalJSON() ([]byte, error) {
	if s.PriceData.TokenRates.Valid() {
		return nil, fmt.Errorf("independent token prices are not supported for asynchronous task snapshots")
	}
	type snapshot TaskBillingSnapshot
	return json.Marshal(snapshot(s))
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
//
//	expression (ExprVars non-empty) -> Eval(expr, ExprVars) USD
//	per-call (PerCallUSD > 0)       -> PerCallUSD x Units x EffectiveRatio
//	token                           -> token billing on Prompt/CompletionTokens
//
// The expression path takes priority when the model has a configured
// expression AND the platform filled ExprVars. A failed expression falls
// through to per-call / token billing (same fallback semantics as
// ComputeUsageQuota).
func TaskPricingQuotaWithPriceData(pd *PriceData, p channel.TaskPricing) int64 {
	if pd.IsExpression() && len(p.ExprVars) > 0 {
		if q, ok := evalExpression(pd, p.ExprVars); ok {
			return q
		}
		// expression failed: fall through to per-call / token
	}
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

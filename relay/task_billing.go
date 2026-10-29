package relay

import (
	"encoding/json"
	"fmt"

	"starhub/common"
	"starhub/pkg/pricingtime"
	"starhub/relay/channel"
)

type TaskBillingSnapshot struct {
	Version   int                 `json:"version,omitempty"`
	PriceData PriceData           `json:"price_data"`
	Estimate  channel.TaskPricing `json:"estimate"`
}

// Version two snapshots carry independent unit prices and complete token facts.
func (s TaskBillingSnapshot) MarshalJSON() ([]byte, error) {
	if s.Version < 0 || s.Version > 2 {
		return nil, fmt.Errorf("unsupported task pricing snapshot version")
	}
	if s.PriceData.HasIndependentTaskPricing() {
		s.Version = 2
	}
	type snapshot TaskBillingSnapshot
	return json.Marshal(snapshot(s))
}
func (s *TaskBillingSnapshot) UnmarshalJSON(raw []byte) error {
	type snapshot TaskBillingSnapshot
	var value snapshot
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	if value.Version < 0 || value.Version > 2 || (value.PriceData.TokenRates.Valid() && value.Version != 2) {
		return fmt.Errorf("unsupported task pricing snapshot version")
	}
	if value.Estimate.PromptTokens < 0 || value.Estimate.CompletionTokens < 0 {
		return fmt.Errorf("invalid task token estimate")
	}
	if value.Version < 2 && !value.PriceData.TokenRates.Valid() {
		value.PriceData.TokenTimeAdjustment = pricingtime.TokenAdjustment{}
	}
	*s = TaskBillingSnapshot(value)
	return nil
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
// A version two token contract takes priority and uses base token rates
// outside independent windows. Other contracts retain legacy precedence.
// The expression path takes priority when the model has a configured
// expression AND the platform filled ExprVars. A failed expression falls
// through to per-call / token billing (same fallback semantics as
// ComputeUsageQuota).
func TaskPricingQuotaWithPriceData(pd *PriceData, p channel.TaskPricing) int64 {
	if pd.HasIndependentTaskPricing() {
		if p.Usage != nil {
			return tokenUsageQuota(pd, p.Usage)
		}
		return taskTokenReserveQuota(pd, p.PromptTokens, p.CompletionTokens)
	}
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

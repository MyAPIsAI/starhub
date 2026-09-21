package relay

import (
	"errors"
	"log"

	"starhub/common"
	"starhub/model"
	"starhub/pkg/billingexpr"
)

var (
	ErrInsufficientTokenQuota = errors.New("token quota is not enough to process this request")
	ErrInsufficientUserQuota  = errors.New("user quota is not enough to process this request")
)

// estimateReserveTokens pads the prompt estimate: the char-based tokenizer
// under-counts (special tokens, JSON overhead), and the pre-charge must err
// on the safe side.
const estimateReserveTokens = 100

// ComputeQuota prices a plain prompt / completion token pair (kept for
// callers that only need the per-token formula; settlement goes through
// ComputeUsageQuota with the full usage details).
func ComputeQuota(modelName, group string, promptTokens, completionTokens int64) int64 {
	return ComputeUsageQuota(NewPriceData(modelName, group),
		&Usage{PromptTokens: promptTokens, CompletionTokens: completionTokens})
}

// EstimatePreConsumeQuota is the pre-charge for a request: it prices the
// tokenizer-estimated prompt plus the completion budget (max_tokens x n
// choices, or a conservative default). Over-reserving is intentional — the
// settle step refunds the surplus with the real usage.
func EstimatePreConsumeQuota(modelName, group string, body []byte) int64 {
	pd := NewPriceData(modelName, group)
	promptEstimate := EstimateTokens(ExtractPromptText(body)) + estimateReserveTokens
	completionBudget := CompletionBudgetTokens(body)

	if pd.IsExpression() {
		vars := map[string]float64{
			"prompt_tokens":     float64(promptEstimate),
			"completion_tokens": float64(completionBudget),
			"total_tokens":      float64(promptEstimate + completionBudget),
		}
		usd, err := billingexpr.Eval(pd.PriceExpr, vars)
		if err != nil {
			log.Printf("[%s] model %s expression %q failed at pre-charge (%v), falling back to token estimate",
				common.QuotaSaturation, modelName, pd.PriceExpr, err)
			return tokenUsageQuota(pd, &Usage{
				PromptTokens:     promptEstimate,
				CompletionTokens: completionBudget,
			})
		}
		return priceUSDToQuota(pd, usd)
	}
	if pd.IsPerCall() {
		return priceUSDToQuota(pd, pd.ModelPriceUSD)
	}
	return tokenUsageQuota(pd, &Usage{
		PromptTokens:     promptEstimate,
		CompletionTokens: completionBudget,
	})
}

// PreConsumeQuota reserves the estimated cost before the request leaves the
// gateway. The reservation is what stops concurrent overdraft: ten parallel
// requests against a one-dollar balance cannot all pass, because each has
// to hold its estimate up front.
func PreConsumeQuota(userID, tokenID int64, tokenUnlimited bool, modelName, group string, body []byte) (int64, error) {
	pre := EstimatePreConsumeQuota(modelName, group, body)
	if pre < 1 {
		pre = 1
	}
	if err := model.ReserveQuota(userID, tokenID, tokenUnlimited, pre); err != nil {
		switch {
		case errors.Is(err, model.ErrUserQuotaShort):
			return 0, ErrInsufficientUserQuota
		case errors.Is(err, model.ErrTokenQuotaShort):
			return 0, ErrInsufficientTokenQuota
		}
		return 0, err
	}
	return pre, nil
}

// RefundPreConsumedQuota returns the full pre-payment when every attempt of
// a relay failed.
func RefundPreConsumedQuota(tokenID, userID int64, tokenUnlimited bool, amount int64) error {
	return model.ReleaseQuota(userID, tokenID, tokenUnlimited, amount)
}

// PostConsumeQuota settles the pre-payment against the real usage (with
// cache / audio detail) and records the consumption log. usedChannels is
// the retry trail ("channelID->channelID") of the request for
// troubleshooting.
func PostConsumeQuota(userID, tokenID int64, tokenName string, tokenUnlimited bool,
	channelID int64, modelName, group string, usage *Usage, preQuota int64, usedChannels string) error {
	if usage == nil {
		usage = &Usage{}
	}
	actual := ComputeUsageQuota(NewPriceData(modelName, group), usage)
	delta := actual - preQuota
	entry := &model.Log{
		UserID:           userID,
		TokenID:          tokenID,
		TokenName:        tokenName,
		ChannelID:        channelID,
		Model:            modelName,
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		Quota:            actual,
		UsedChannels:     usedChannels,
	}
	return model.SettleQuota(userID, tokenID, tokenUnlimited, delta, actual, entry)
}

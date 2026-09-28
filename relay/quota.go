package relay

import (
	"errors"
	"log"

	"starhub/common"
	"starhub/model"
	"starhub/relay/dto"
	"starhub/setting"
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

// ComputeQuotaWithUserGroup is ComputeQuota with the user's own group, so the
// group x group matrix multiplier can apply.
func ComputeQuotaWithUserGroup(modelName, group, userGroup string, promptTokens, completionTokens int64) int64 {
	return ComputeUsageQuota(NewPriceDataWithUserGroup(modelName, group, userGroup),
		&Usage{PromptTokens: promptTokens, CompletionTokens: completionTokens})
}

// EstimatePreConsumeQuota is the pre-charge for a request: it prices the
// tokenizer-estimated prompt plus the completion budget (max_tokens x n
// choices, or a conservative default). Over-reserving is intentional — the
// settle step refunds the surplus with the real usage.
func EstimatePreConsumeQuota(modelName, group string, body []byte) int64 {
	pd := NewPriceData(modelName, group)
	promptEstimate := dto.EstimateTokens(ExtractPromptText(body)) + estimateReserveTokens
	completionBudget := CompletionBudgetTokens(body)
	return estimatePreConsume(pd, promptEstimate, completionBudget)
}

// EstimatePreConsumeQuotaWithUserGroup is EstimatePreConsumeQuota with the
// user's own group for the group x group matrix multiplier.
func EstimatePreConsumeQuotaWithUserGroup(modelName, group, userGroup string, body []byte) int64 {
	pd := NewPriceDataWithUserGroup(modelName, group, userGroup)
	promptEstimate := dto.EstimateTokens(ExtractPromptText(body)) + estimateReserveTokens
	completionBudget := CompletionBudgetTokens(body)
	return estimatePreConsume(pd, promptEstimate, completionBudget)
}

// EstimateIRPreConsumeQuota is the IR-based pre-charge used by inbound
// protocols other than OpenAI (the completion budget is the IR max_tokens
// or the conservative default).
func EstimateIRPreConsumeQuota(modelName, group string, ir *dto.ChatRequest) int64 {
	pd := NewPriceData(modelName, group)
	promptEstimate := dto.EstimateTokens(ir.PromptText()) + estimateReserveTokens
	completionBudget := ir.MaxTokens
	if completionBudget <= 0 {
		completionBudget = common.DefaultCompletionBudgetTokens
	}
	return estimatePreConsume(pd, promptEstimate, completionBudget)
}

// EstimateIRPreConsumeQuotaWithUserGroup is EstimateIRPreConsumeQuota with
// the user's own group for the group x group matrix multiplier.
func EstimateIRPreConsumeQuotaWithUserGroup(modelName, group, userGroup string, ir *dto.ChatRequest) int64 {
	pd := NewPriceDataWithUserGroup(modelName, group, userGroup)
	promptEstimate := dto.EstimateTokens(ir.PromptText()) + estimateReserveTokens
	completionBudget := ir.MaxTokens
	if completionBudget <= 0 {
		completionBudget = common.DefaultCompletionBudgetTokens
	}
	return estimatePreConsume(pd, promptEstimate, completionBudget)
}

// EstimatePreConsumeTokens prices an explicit prompt-token estimate with a
// zero completion budget (embeddings / rerank / audio inputs: the prompt
// side is the whole bill).
func EstimatePreConsumeTokens(modelName, group string, promptTokens int64) int64 {
	return estimatePreConsume(NewPriceData(modelName, group), promptTokens, 0)
}

// estimatePreConsume prices one prompt/budget estimate with the expression
// / per-call / token priority chain.
func estimatePreConsume(pd *PriceData, promptEstimate, completionBudget int64) int64 {
	if pd.IsExpression() {
		vars := map[string]float64{
			"prompt_tokens":     float64(promptEstimate),
			"completion_tokens": float64(completionBudget),
			"total_tokens":      float64(promptEstimate + completionBudget),
		}
		if q, ok := evalExpression(pd, vars); ok {
			return q
		}
		// expression failed: fall back to token estimate
		return tokenUsageQuota(pd, &Usage{
			PromptTokens:     promptEstimate,
			CompletionTokens: completionBudget,
		})
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
	return reservePreConsumed(userID, tokenID, tokenUnlimited, pre)
}

// PreConsumeQuotaWithUserGroup is PreConsumeQuota with the user's own group
// for the group x group matrix multiplier.
func PreConsumeQuotaWithUserGroup(userID, tokenID int64, tokenUnlimited bool, modelName, group, userGroup string, body []byte) (int64, error) {
	pre := EstimatePreConsumeQuotaWithUserGroup(modelName, group, userGroup, body)
	return reservePreConsumed(userID, tokenID, tokenUnlimited, pre)
}

// PreConsumeIRQuota reserves the IR-based chat estimate (non-OpenAI inbound
// protocols).
func PreConsumeIRQuota(userID, tokenID int64, tokenUnlimited bool, modelName, group string, ir *dto.ChatRequest) (int64, error) {
	pre := EstimateIRPreConsumeQuota(modelName, group, ir)
	return reservePreConsumed(userID, tokenID, tokenUnlimited, pre)
}

// PreConsumeIRQuotaWithUserGroup is PreConsumeIRQuota with the user's own
// group for the group x group matrix multiplier.
func PreConsumeIRQuotaWithUserGroup(userID, tokenID int64, tokenUnlimited bool, modelName, group, userGroup string, ir *dto.ChatRequest) (int64, error) {
	pre := EstimateIRPreConsumeQuotaWithUserGroup(modelName, group, userGroup, ir)
	return reservePreConsumed(userID, tokenID, tokenUnlimited, pre)
}

// PreConsumeQuotaAmount reserves an explicit amount. The image / audio /
// task flows compute their estimates with mode-specific multipliers and
// hand the number to the same overdraft-safe reservation.
func PreConsumeQuotaAmount(userID, tokenID int64, tokenUnlimited bool, amount int64) (int64, error) {
	return reservePreConsumed(userID, tokenID, tokenUnlimited, amount)
}

func reservePreConsumed(userID, tokenID int64, tokenUnlimited bool, pre int64) (int64, error) {
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
	return PostConsumeQuotaWithPrice(NewPriceData(modelName, group), userID, tokenID, tokenName,
		tokenUnlimited, channelID, modelName, usage, preQuota, usedChannels)
}

// PostConsumeQuotaWithUserGroup is PostConsumeQuota with the user's own group
// for the group x group matrix multiplier.
func PostConsumeQuotaWithUserGroup(userID, tokenID int64, tokenName string, tokenUnlimited bool,
	channelID int64, modelName, group, userGroup string, usage *Usage, preQuota int64, usedChannels string) error {
	return PostConsumeQuotaWithPrice(NewPriceDataWithUserGroup(modelName, group, userGroup), userID, tokenID, tokenName,
		tokenUnlimited, channelID, modelName, usage, preQuota, usedChannels)
}

// PostConsumeQuotaWithPrice is the settlement entry for relay modes that
// carry special multipliers (image resolution x count, video seconds x n):
// the caller resolves its own PriceData and this function only balances
// the books.
func PostConsumeQuotaWithPrice(pd *PriceData, userID, tokenID int64, tokenName string, tokenUnlimited bool,
	channelID int64, modelName string, usage *Usage, preQuota int64, usedChannels string) error {
	if usage == nil {
		usage = &Usage{}
	}
	actual := ComputeUsageQuota(pd, usage)
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
	if err := model.SettleQuota(userID, tokenID, tokenUnlimited, delta, actual, entry); err != nil {
		return err
	}
	// Affiliate rebate: the inviter gets a percentage of the collected spend.
	// The rebate is best-effort (a failure is logged, not propagated) so a
	// rebate glitch never fails the relay settlement.
	applyAffiliateRebate(userID, entry.Quota)
	return nil
}

// applyAffiliateRebate credits the inviter's quota with the rebate ratio of
// the invitee's actual spend. It is a no-op when the user has no inviter or
// the rebate ratio is 0.
func applyAffiliateRebate(userID, actual int64) {
	if actual <= 0 {
		return
	}
	ratio := setting.AffRebateRatioValue()
	if ratio <= 0 {
		return
	}
	user, err := model.GetUserByID(userID)
	if err != nil || user.InviterID <= 0 {
		return
	}
	rebate := common.QuotaFromFloat64(float64(actual) * ratio)
	if rebate <= 0 {
		return
	}
	if err := model.IncreaseUserQuota(user.InviterID, rebate); err != nil {
		log.Printf("affiliate rebate to user %d failed: %v", user.InviterID, err)
	}
}

// PostConsumeQuotaAmount settles an explicit actual amount (unit-based
// bills such as tasks, where the actual is computed from platform pricing
// rather than a usage report).
func PostConsumeQuotaAmount(userID, tokenID int64, tokenName string, tokenUnlimited bool,
	channelID int64, modelName string, actual, preQuota int64, usedChannels string) error {
	delta := actual - preQuota
	entry := &model.Log{
		UserID:       userID,
		TokenID:      tokenID,
		TokenName:    tokenName,
		ChannelID:    channelID,
		Model:        modelName,
		Quota:        actual,
		UsedChannels: usedChannels,
	}
	if err := model.SettleQuota(userID, tokenID, tokenUnlimited, delta, actual, entry); err != nil {
		return err
	}
	applyAffiliateRebate(userID, entry.Quota)
	return nil
}

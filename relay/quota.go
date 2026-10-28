package relay

import (
	"errors"
	"log"

	"starhub/common"
	"starhub/model"
	"starhub/relay/dto"
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
	return EstimatePreConsumeQuotaWithPriceData(pd, body)
}

func EstimatePreConsumeQuotaWithPriceData(pd *PriceData, body []byte) int64 {
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
	return EstimateIRPreConsumeQuotaWithPriceData(pd, ir)
}

func EstimateIRPreConsumeQuotaWithPriceData(pd *PriceData, ir *dto.ChatRequest) int64 {
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
	channelID int64, modelName, group string, usage *Usage, preQuota int64, usedChannels string, durationMs int64) error {
	return PostConsumeQuotaWithPrice(NewPriceData(modelName, group), userID, tokenID, tokenName,
		tokenUnlimited, channelID, modelName, usage, preQuota, usedChannels, durationMs)
}

// PostConsumeQuotaWithUserGroup is PostConsumeQuota with the user's own group
// for the group x group matrix multiplier.
// An optional requestedModel preserves the client model in the usage log.
func PostConsumeQuotaWithUserGroup(userID, tokenID int64, tokenName string, tokenUnlimited bool,
	channelID int64, modelName, group, userGroup string, usage *Usage, preQuota int64, usedChannels string, durationMs int64, requestedModel ...string) error {
	return PostConsumeQuotaWithPrice(NewPriceDataWithUserGroup(modelName, group, userGroup), userID, tokenID, tokenName,
		tokenUnlimited, channelID, modelName, usage, preQuota, usedChannels, durationMs, requestedModel...)
}

// PostConsumeQuotaWithPrice is the settlement entry for relay modes that
// carry special multipliers (image resolution x count, video seconds x n):
// the caller resolves its own PriceData and this function only balances
// the books.
func PostConsumeQuotaWithPrice(pd *PriceData, userID, tokenID int64, tokenName string, tokenUnlimited bool,
	channelID int64, modelName string, usage *Usage, preQuota int64, usedChannels string, durationMs int64, requestedModel ...string) error {
	if usage == nil {
		usage = &Usage{}
	}
	status := usage.RequestStatus
	if status == 0 {
		status = 1
	}
	if status < 1 || status > 3 {
		return errors.New("unsupported settlement log status")
	}
	actual := ComputeUsageQuota(pd, usage)
	delta := actual - preQuota
	originalModel := modelName
	if len(requestedModel) > 0 && requestedModel[0] != "" {
		originalModel = requestedModel[0]
	}
	entry := &model.Log{
		UserID:           userID,
		TokenID:          tokenID,
		TokenName:        tokenName,
		ChannelID:        channelID,
		Model:            modelName,
		RequestedModel:   originalModel,
		Protocol:         usage.RequestProtocol,
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		Quota:            actual,
		UsedChannels:     usedChannels,
		DurationMs:       durationMs,
		Status:           status,
	}
	return settleDurableQuota(userID, tokenID, tokenUnlimited, delta, actual, entry)
}

// applyAffiliateRebate attempts immediate delivery; durable pending work is
// independent of the already-committed customer settlement.
func applyAffiliateRebate(logID int64) {
	if err := model.DeliverAffiliateRebate(logID); err != nil {
		log.Printf("affiliate rebate for log %d failed: %v", logID, err)
	}
}

// PostConsumeQuotaAmount settles an explicit actual amount (unit-based
// bills such as tasks, where the actual is computed from platform pricing
// rather than a usage report).
// An optional requestedModel preserves the client model in the usage log.
func PostConsumeQuotaAmount(userID, tokenID int64, tokenName string, tokenUnlimited bool,
	channelID int64, modelName string, actual, preQuota int64, usedChannels string, durationMs int64, requestedModel ...string) error {
	return PostConsumeQuotaAmountWithStatus(userID, tokenID, tokenName, tokenUnlimited,
		channelID, modelName, actual, preQuota, usedChannels, durationMs, 1, requestedModel...)
}

// PostConsumeQuotaAmountWithStatus keeps the outcome and quota settlement in
// the same transaction. Outcome is independent of the collected amount.
func PostConsumeQuotaAmountWithStatus(userID, tokenID int64, tokenName string, tokenUnlimited bool,
	channelID int64, modelName string, actual, preQuota int64, usedChannels string, durationMs int64, status int, requestedModel ...string) error {
	originalModel := modelName
	if len(requestedModel) > 0 && requestedModel[0] != "" {
		originalModel = requestedModel[0]
	}
	return PostConsumeQuotaAmountWithMetadata(userID, tokenID, tokenName, tokenUnlimited,
		channelID, modelName, actual, preQuota, usedChannels, durationMs, status, originalModel, "")
}

// PostConsumeQuotaAmountWithMetadata records gateway-owned request metadata.
func PostConsumeQuotaAmountWithMetadata(userID, tokenID int64, tokenName string, tokenUnlimited bool,
	channelID int64, modelName string, actual, preQuota int64, usedChannels string, durationMs int64, status int, originalModel, protocol string) error {
	if status < 1 || status > 3 {
		return errors.New("unsupported settlement log status")
	}
	delta := actual - preQuota
	entry := &model.Log{
		UserID:         userID,
		TokenID:        tokenID,
		TokenName:      tokenName,
		ChannelID:      channelID,
		Model:          modelName,
		RequestedModel: originalModel,
		Protocol:       protocol,
		Quota:          actual,
		UsedChannels:   usedChannels,
		DurationMs:     durationMs,
		Status:         status,
	}
	return settleDurableQuota(userID, tokenID, tokenUnlimited, delta, actual, entry)
}

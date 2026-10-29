package relay

import (
	"starhub/common"
	"starhub/relay/channel"
	"starhub/relay/dto"
)

// A configured schedule uses base token rates between its explicit windows.
func (pd *PriceData) HasIndependentTaskPricing() bool {
	return pd.TokenRates.Valid() || pd.TokenTimeAdjustment.Timezone != ""
}

func taskTokenReserveQuota(pd *PriceData, input, output int64) int64 {
	if pd.TokenRates.Valid() {
		return tokenReserveQuota(pd, input, output)
	}
	usage := &dto.Usage{PromptTokens: input, CompletionTokens: output}
	if pd.CacheRatio > 1 && pd.CacheRatio >= pd.AudioRatio {
		usage.PromptTokensDetails = &dto.PromptTokensDetails{CachedTokens: input}
	} else if pd.AudioRatio > 1 {
		usage.PromptTokensDetails = &dto.PromptTokensDetails{AudioTokens: input}
	}
	if pd.AudioRatio > pd.CompletionRatio {
		usage.CompletionTokensDetails = &dto.CompletionTokensDetails{AudioTokens: output}
	}
	return tokenUsageQuota(pd, usage)
}

// A reservation is a gateway estimate, never a claimed provider measurement.
// Unknown token classes reserve at the most expensive captured unit price.
func TaskTokenEstimate(request *dto.TaskRequest) channel.TaskPricing {
	n := int64(max(request.N, 1))
	output := request.MaxOutputTokens
	if output <= 0 {
		output = common.DefaultCompletionBudgetTokens
	}
	return channel.TaskPricing{PromptTokens: (dto.EstimateTokens(request.Prompt) + 100) * n, CompletionTokens: output * n}
}

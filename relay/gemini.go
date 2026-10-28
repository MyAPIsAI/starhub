package relay

import (
	"fmt"

	"starhub/common"
	"starhub/relay/dto"
)

func ValidateGeminiRequest(req *dto.GeminiRequest) error {
	if limit := req.GenerationConfig.MaxOutputTokens; limit != nil && *limit > common.MaxChatMaxTokens {
		return fmt.Errorf("maxOutputTokens must not exceed %d", common.MaxChatMaxTokens)
	}
	return nil
}

func EstimateGeminiPreConsumeQuota(modelName, group, userGroup string, req *dto.GeminiRequest) int64 {
	output := int64(common.DefaultCompletionBudgetTokens)
	if req.GenerationConfig.MaxOutputTokens != nil {
		output = *req.GenerationConfig.MaxOutputTokens
	}
	return estimatePreConsume(NewPriceDataWithUserGroup(modelName, group, userGroup), req.InputTokens+estimateReserveTokens, output)
}

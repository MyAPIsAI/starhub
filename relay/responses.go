package relay

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"starhub/common"
	"starhub/relay/dto"
)

// ValidateResponsesRequest bounds accepted foreground requests before quota
// admission. Provider-owned references need a separate scoped lifecycle.
func ValidateResponsesRequest(req *dto.ResponsesRequest) error {
	if req.MaxOutputTokens != nil && *req.MaxOutputTokens > common.MaxChatMaxTokens {
		return fmt.Errorf("max_output_tokens must not exceed %d", common.MaxChatMaxTokens)
	}
	if req.Background && (req.Stream || req.Store == nil || !*req.Store) {
		return errors.New("background Responses require explicit store:true and stream:false")
	}
	if len(req.Conversation) > 0 && !bytes.Equal(bytes.TrimSpace(req.Conversation), []byte("null")) {
		return errors.New("stored Responses references are not available yet; send complete input history")
	}
	if req.HasFiles {
		return errors.New("provider file inputs are not available yet; send text or image URLs")
	}
	for _, raw := range req.Tools {
		var tool struct {
			Type  string            `json:"type"`
			Tools []json.RawMessage `json:"tools"`
		}
		if err := json.Unmarshal(raw, &tool); err != nil {
			return err
		}
		switch tool.Type {
		case "function", "custom":
		case "namespace":
			for _, nested := range tool.Tools {
				var item struct {
					Type string `json:"type"`
				}
				if err := json.Unmarshal(nested, &item); err != nil {
					return err
				}
				if item.Type != "function" && item.Type != "custom" {
					return errors.New("tool namespaces support function and custom tools only")
				}
			}
		default:
			return errors.New("this endpoint currently supports function, custom and namespace tools")
		}
	}
	return nil
}

func EstimateResponsesPreConsumeQuota(modelName, group, userGroup string, req *dto.ResponsesRequest) int64 {
	return EstimateResponsesPreConsumeQuotaWithPriceData(NewPriceDataWithUserGroup(modelName, group, userGroup), req)
}

func EstimateResponsesPreConsumeQuotaWithPriceData(pd *PriceData, req *dto.ResponsesRequest) int64 {
	output := int64(common.DefaultCompletionBudgetTokens)
	if req.MaxOutputTokens != nil {
		output = *req.MaxOutputTokens
	}
	return estimatePreConsume(pd, req.InputTokens+estimateReserveTokens, output)
}

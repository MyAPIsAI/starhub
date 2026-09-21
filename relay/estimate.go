package relay

import (
	"encoding/json"
	"strings"

	"starhub/common"
	"starhub/relay/dto"
)

// EstimateTokens is the char-based tokenizer approximation (see
// dto.EstimateTokens).
func EstimateTokens(text string) int64 {
	return dto.EstimateTokens(text)
}

func EstimateUsage(requestBody []byte, streamedContent string) *Usage {
	return &Usage{
		PromptTokens:     EstimateTokens(ExtractPromptText(requestBody)),
		CompletionTokens: EstimateTokens(streamedContent),
	}
}

// EstimateIRUsage builds the fallback usage of a converted chat request
// when the upstream report is missing (IR-based, protocol independent).
func EstimateIRUsage(ir *dto.ChatRequest, streamedContent string) *Usage {
	return &Usage{
		PromptTokens:     EstimateTokens(ir.PromptText()),
		CompletionTokens: EstimateTokens(streamedContent),
	}
}

// RequestBudget is the billing-relevant request shape used both for
// validation and for the pre-charge estimate.
type RequestBudget struct {
	MaxTokens int64 // max_tokens / max_completion_tokens, 0 when unset
	Choices   int64 // "n", 0/absent means 1
}

// ExtractRequestBudget pulls the completion-token budget multipliers out of
// a chat request body. Unknown / malformed fields degrade to the defaults
// (the strict validation happens in ValidateChatRequest).
func ExtractRequestBudget(body []byte) RequestBudget {
	var req struct {
		MaxTokens           *int64 `json:"max_tokens"`
		MaxCompletionTokens *int64 `json:"max_completion_tokens"`
		N                   *int64 `json:"n"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return RequestBudget{}
	}
	var budget RequestBudget
	if req.MaxTokens != nil {
		budget.MaxTokens = *req.MaxTokens
	} else if req.MaxCompletionTokens != nil {
		budget.MaxTokens = *req.MaxCompletionTokens
	}
	if req.N != nil {
		budget.Choices = *req.N
	}
	return budget
}

// CompletionBudgetTokens is the number of completion tokens the pre-charge
// reserves for: max_tokens x n choices when the caller pinned them, or the
// conservative default when the model decides.
func CompletionBudgetTokens(body []byte) int64 {
	budget := ExtractRequestBudget(body)
	maxTokens := budget.MaxTokens
	if maxTokens <= 0 {
		maxTokens = common.DefaultCompletionBudgetTokens
	}
	choices := budget.Choices
	if choices <= 0 {
		choices = 1
	}
	return maxTokens * choices
}

func ExtractPromptText(body []byte) string {
	var req struct {
		Messages []struct {
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return ""
	}
	parts := make([]string, 0, len(req.Messages))
	for _, m := range req.Messages {
		if text := contentText(m.Content); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, " ")
}

func contentText(raw json.RawMessage) string {
	return dto.ContentText(raw)
}

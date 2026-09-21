package relay

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"starhub/common"
)

func EstimateTokens(text string) int64 {
	n := utf8.RuneCountInString(text)
	if n == 0 {
		return 0
	}
	return int64((n + 3) / 4)
}

func EstimateUsage(requestBody []byte, streamedContent string) *Usage {
	return &Usage{
		PromptTokens:     EstimateTokens(ExtractPromptText(requestBody)),
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
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err == nil {
		var sb strings.Builder
		for _, p := range parts {
			if p.Text != "" {
				sb.WriteString(p.Text)
				sb.WriteByte(' ')
			}
		}
		return strings.TrimSpace(sb.String())
	}
	return ""
}

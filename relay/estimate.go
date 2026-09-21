package relay

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
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

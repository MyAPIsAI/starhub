package middleware

import (
	"encoding/json"

	"starhub/relay/dto"
)

// smartToolBlockTokens adds tool inputs and nested tool-result text omitted by
// the ordinary message-text extractor. Image bytes are not text tokens.
func smartToolBlockTokens(raw json.RawMessage) int64 {
	var blocks []struct {
		Type      string          `json:"type"`
		ID        string          `json:"id"`
		Name      string          `json:"name"`
		Input     json.RawMessage `json:"input"`
		ToolUseID string          `json:"tool_use_id"`
		Content   any             `json:"content"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return 0
	}
	var resultTokens func(any) int64
	resultTokens = func(content any) int64 {
		if text, ok := content.(string); ok {
			return dto.EstimateTokens(text)
		}
		var total int64
		if children, ok := content.([]any); ok {
			for _, child := range children {
				block, ok := child.(map[string]any)
				if !ok {
					continue
				}
				kind, _ := block["type"].(string)
				switch kind {
				case "text":
					text, _ := block["text"].(string)
					total += dto.EstimateTokens(text)
				case "tool_result":
					id, _ := block["tool_use_id"].(string)
					total += dto.EstimateTokens(id) + resultTokens(block["content"])
				case "tool_use":
					id, _ := block["id"].(string)
					name, _ := block["name"].(string)
					input, _ := json.Marshal(block["input"])
					total += dto.EstimateTokens(id + " " + name + " " + string(input))
				}
			}
		}
		return total
	}
	var total int64
	for _, block := range blocks {
		switch block.Type {
		case "tool_use":
			total += dto.EstimateTokens(block.ID + " " + block.Name + " " + string(block.Input))
		case "tool_result":
			total += dto.EstimateTokens(block.ToolUseID) + resultTokens(block.Content)
		}
	}
	return total
}

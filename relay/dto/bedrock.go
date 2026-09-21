package dto

import (
	"encoding/json"
	"strings"
)

// ---------------------------------------------------------------------------
// IR -> AWS Bedrock converse (aws channels)
// ---------------------------------------------------------------------------

type bedrockContentBlock struct {
	Text       string          `json:"text,omitempty"`
	Image      json.RawMessage `json:"image,omitempty"`
	ToolUse    *bedrockToolUse `json:"toolUse,omitempty"`
	ToolResult *struct {
		ToolUseID string          `json:"toolUseId"`
		Content   json.RawMessage `json:"content"`
	} `json:"toolResult,omitempty"`
}

type bedrockToolUse struct {
	ToolUseID string          `json:"toolUseId"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
}

// BuildBedrockChatRequest serializes IR into a Bedrock converse body.
func BuildBedrockChatRequest(ir *ChatRequest) ([]byte, error) {
	var out struct {
		System          []map[string]any `json:"system,omitempty"`
		Messages        []map[string]any `json:"messages"`
		InferenceConfig map[string]any   `json:"inferenceConfig,omitempty"`
		ToolConfig      map[string]any   `json:"toolConfig,omitempty"`
	}
	for _, m := range ir.Messages {
		switch m.Role {
		case "system":
			if m.Text != "" {
				out.System = append(out.System, map[string]any{"text": m.Text})
			}
			continue
		}
		role := m.Role
		if role == "tool" {
			role = "user" // Bedrock tool results ride on user turns
		}
		blocks := []bedrockContentBlock{}
		if m.Text != "" {
			blocks = append(blocks, bedrockContentBlock{Text: m.Text})
		}
		for _, tc := range m.ToolCalls {
			input := json.RawMessage(`{}`)
			if tc.Arguments != "" {
				var parsed any
				if json.Unmarshal([]byte(tc.Arguments), &parsed) == nil {
					input = mustJSON(parsed)
				}
			}
			blocks = append(blocks, bedrockContentBlock{
				ToolUse: &bedrockToolUse{ToolUseID: tc.ID, Name: tc.Name, Input: input},
			})
		}
		if m.ToolCallID != "" {
			inner := mustJSON([]map[string]any{{"text": orDefault(m.Text, " ")}})
			blocks = append(blocks, bedrockContentBlock{
				ToolResult: &struct {
					ToolUseID string          `json:"toolUseId"`
					Content   json.RawMessage `json:"content"`
				}{ToolUseID: m.ToolCallID, Content: inner},
			})
		}
		if len(blocks) == 0 {
			blocks = append(blocks, bedrockContentBlock{Text: " "})
		}
		out.Messages = append(out.Messages, map[string]any{
			"role":    role,
			"content": blocks,
		})
	}
	cfg := map[string]any{}
	if ir.MaxTokens > 0 {
		cfg["maxTokens"] = ir.MaxTokens
	}
	if ir.Temperature != nil {
		cfg["temperature"] = *ir.Temperature
	}
	if ir.TopP != nil {
		cfg["topP"] = *ir.TopP
	}
	if len(cfg) > 0 {
		out.InferenceConfig = cfg
	}
	if len(ir.Tools) > 0 {
		specs := make([]map[string]any, 0, len(ir.Tools))
		for _, t := range ir.Tools {
			specs = append(specs, map[string]any{
				"toolSpec": map[string]any{
					"name":        t.Name,
					"description": t.Description,
					"inputSchema": map[string]any{"json": orEmptySchema(t.Parameters)},
				},
			})
		}
		out.ToolConfig = map[string]any{"tools": specs}
	}
	return json.Marshal(out)
}

// ParseBedrockChatResponse parses a converse response into IR.
func ParseBedrockChatResponse(body []byte) (*ChatResponse, error) {
	var resp struct {
		Output struct {
			Message struct {
				Content []bedrockContentBlock `json:"content"`
			} `json:"message"`
		} `json:"output"`
		StopReason string `json:"stopReason"`
		Usage      *struct {
			InputTokens  int64 `json:"inputTokens"`
			OutputTokens int64 `json:"outputTokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	ir := &ChatResponse{}
	if resp.Usage != nil {
		ir.Usage = &Usage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
		}
	}
	switch resp.StopReason {
	case "end_turn", "stop_sequence":
		ir.FinishReason = "stop"
	case "max_tokens":
		ir.FinishReason = "length"
	case "tool_use":
		ir.FinishReason = "tool_calls"
	default:
		ir.FinishReason = resp.StopReason
	}
	var sb strings.Builder
	for _, b := range resp.Output.Message.Content {
		if b.Text != "" {
			sb.WriteString(b.Text)
		}
		if b.ToolUse != nil {
			ir.ToolCalls = append(ir.ToolCalls, ToolCall{
				ID:        b.ToolUse.ToolUseID,
				Name:      b.ToolUse.Name,
				Arguments: string(b.ToolUse.Input),
			})
		}
	}
	ir.Text = sb.String()
	return ir, nil
}

// ParseBedrockStreamEvent converts one converse-stream JSON payload (the
// payload of an AWS eventstream "chunk" event) into an IR stream event.
func ParseBedrockStreamEvent(payload []byte) (*ChatStreamEvent, error) {
	var ev struct {
		Type  string `json:"type"` // messageStart, contentBlockDelta, messageStop, metadata...
		Delta *struct {
			Text       string `json:"text"`
			StopReason string `json:"stopReason"`
		} `json:"delta"`
		Usage *struct {
			InputTokens  int64 `json:"inputTokens"`
			OutputTokens int64 `json:"outputTokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(payload, &ev); err != nil {
		return nil, err
	}
	out := &ChatStreamEvent{}
	switch ev.Type {
	case "contentBlockDelta":
		if ev.Delta != nil {
			out.TextDelta = ev.Delta.Text
		}
	case "messageDelta", "messageStop":
		if ev.Delta != nil && ev.Delta.StopReason != "" {
			switch ev.Delta.StopReason {
			case "end_turn", "stop_sequence":
				out.FinishReason = "stop"
			case "max_tokens":
				out.FinishReason = "length"
			default:
				out.FinishReason = ev.Delta.StopReason
			}
		}
	}
	if ev.Usage != nil && (ev.Usage.InputTokens > 0 || ev.Usage.OutputTokens > 0) {
		out.Usage = &Usage{
			PromptTokens:     ev.Usage.InputTokens,
			CompletionTokens: ev.Usage.OutputTokens,
		}
	}
	return out, nil
}

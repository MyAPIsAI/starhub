package dto

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ---------------------------------------------------------------------------
// Claude -> IR (inbound /v1/messages and upstream Claude channels)
// ---------------------------------------------------------------------------

type claudeWireBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Source    json.RawMessage `json:"source,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
}

// ParseClaudeChatRequest parses a Claude /v1/messages body into IR.
func ParseClaudeChatRequest(body []byte) (*ChatRequest, error) {
	var req struct {
		Model         string           `json:"model"`
		Messages      []claudeWireMsg  `json:"messages"`
		System        json.RawMessage  `json:"system"`
		MaxTokens     *int64           `json:"max_tokens"`
		Temperature   *float64         `json:"temperature"`
		TopP          *float64         `json:"top_p"`
		StopSequences []string         `json:"stop_sequences"`
		Stream        bool             `json:"stream"`
		Tools         []claudeWireTool `json:"tools"`
		ToolChoice    json.RawMessage  `json:"tool_choice"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	ir := &ChatRequest{
		Model:       req.Model,
		MaxTokens:   deref(req.MaxTokens),
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stop:        req.StopSequences,
		Stream:      req.Stream,
		ToolChoice:  req.ToolChoice,
	}
	if system := claudeSystemText(req.System); system != "" {
		ir.Messages = append(ir.Messages, ChatMessage{Role: "system", Text: system})
	}
	for _, t := range req.Tools {
		ir.Tools = append(ir.Tools, Tool{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.InputSchema,
		})
	}
	for i, m := range req.Messages {
		msg, err := parseClaudeMessage(m)
		if err != nil {
			return nil, fmt.Errorf("messages[%d]: %w", i, err)
		}
		ir.Messages = append(ir.Messages, msg)
	}
	return ir, nil
}

type claudeWireMsg struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type claudeWireTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

func parseClaudeMessage(m claudeWireMsg) (ChatMessage, error) {
	msg := ChatMessage{Role: m.Role}
	if len(m.Content) == 0 {
		return msg, nil
	}
	var text string
	if err := json.Unmarshal(m.Content, &text); err == nil {
		msg.Text = text
		return msg, nil
	}
	var blocks []claudeWireBlock
	if err := json.Unmarshal(m.Content, &blocks); err != nil {
		return msg, fmt.Errorf("invalid content: %w", err)
	}
	var sb strings.Builder
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if b.Text != "" {
				sb.WriteString(b.Text)
				sb.WriteByte(' ')
			}
		case "tool_use":
			args := "{}"
			if len(b.Input) > 0 {
				args = string(b.Input)
			}
			msg.ToolCalls = append(msg.ToolCalls, ToolCall{
				ID: b.ID, Name: b.Name, Arguments: args,
			})
		case "tool_result":
			msg.Role = "tool"
			msg.ToolCallID = b.ToolUseID
			if text := claudeToolResultText(b.Content); text != "" {
				sb.WriteString(text)
				sb.WriteByte(' ')
			}
		case "image":
			if img, ok := parseClaudeImageSource(b.Source); ok {
				msg.Images = append(msg.Images, img)
			}
		}
	}
	msg.Text = strings.TrimSpace(sb.String())
	return msg, nil
}

func claudeToolResultText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []claudeWireBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var sb strings.Builder
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				sb.WriteString(b.Text)
				sb.WriteByte(' ')
			}
		}
		return strings.TrimSpace(sb.String())
	}
	return ""
}

func parseClaudeImageSource(raw json.RawMessage) (ImageInput, bool) {
	if len(raw) == 0 {
		return ImageInput{}, false
	}
	var src struct {
		Type      string `json:"type"`
		URL       string `json:"url"`
		MediaType string `json:"media_type"`
		Data      string `json:"data"`
	}
	if err := json.Unmarshal(raw, &src); err != nil {
		return ImageInput{}, false
	}
	switch src.Type {
	case "base64":
		return ImageInput{Base64: src.Data, MediaType: src.MediaType}, true
	case "url":
		return ImageInput{URL: src.URL, MediaType: src.MediaType}, true
	}
	return ImageInput{}, false
}

func claudeSystemText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []claudeWireBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var sb strings.Builder
		for _, b := range blocks {
			if b.Text != "" {
				sb.WriteString(b.Text)
				sb.WriteByte(' ')
			}
		}
		return strings.TrimSpace(sb.String())
	}
	return ""
}

// ---------------------------------------------------------------------------
// IR -> Claude (claude-in clients served by claude channels; IR -> claude out)
// ---------------------------------------------------------------------------

// BuildClaudeChatRequest serializes IR into a Claude /v1/messages body.
func BuildClaudeChatRequest(ir *ChatRequest) ([]byte, error) {
	var out struct {
		Model         string           `json:"model"`
		System        json.RawMessage  `json:"system,omitempty"`
		Messages      []claudeWireMsg  `json:"messages"`
		MaxTokens     int64            `json:"max_tokens"`
		Temperature   *float64         `json:"temperature,omitempty"`
		TopP          *float64         `json:"top_p,omitempty"`
		StopSequences []string         `json:"stop_sequences,omitempty"`
		Stream        bool             `json:"stream,omitempty"`
		Tools         []claudeWireTool `json:"tools,omitempty"`
		ToolChoice    json.RawMessage  `json:"tool_choice,omitempty"`
	}
	out.Model = ir.Model
	out.MaxTokens = ir.MaxTokens
	if out.MaxTokens <= 0 {
		// Claude requires max_tokens; mirror the conservative budget.
		out.MaxTokens = 4096
	}
	out.Temperature = ir.Temperature
	out.TopP = ir.TopP
	out.StopSequences = ir.Stop
	out.Stream = ir.Stream
	for _, m := range ir.Messages {
		if m.Role == "system" {
			if len(out.System) == 0 && m.Text != "" {
				out.System = mustJSON(m.Text)
			}
			continue
		}
		out.Messages = append(out.Messages, buildClaudeMessage(m))
	}
	for _, t := range ir.Tools {
		out.Tools = append(out.Tools, claudeWireTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: orEmptySchema(t.Parameters),
		})
	}
	if len(ir.ToolChoice) > 0 {
		out.ToolChoice = convertToolChoiceToClaude(ir.ToolChoice)
	}
	return json.Marshal(out)
}

func buildClaudeMessage(m ChatMessage) claudeWireMsg {
	blocks := []map[string]any{}
	// A tool result turn leads with its tool_result block (Claude requires
	// tool_result blocks before any text in the same message).
	if m.ToolCallID != "" {
		inner := m.Text
		var innerAny any = inner
		if inner == "" {
			innerAny = " "
		}
		blocks = append(blocks, map[string]any{
			"type":        "tool_result",
			"tool_use_id": m.ToolCallID,
			"content":     innerAny,
		})
	}
	if m.Text != "" && m.ToolCallID == "" {
		blocks = append(blocks, map[string]any{"type": "text", "text": m.Text})
	}
	for _, img := range m.Images {
		if img.URL != "" {
			blocks = append(blocks, map[string]any{
				"type": "image",
				"source": map[string]any{
					"type": "url",
					"url":  img.URL,
				},
			})
			continue
		}
		src := map[string]any{
			"type": "base64",
			"data": img.Base64,
		}
		if img.MediaType != "" {
			src["media_type"] = img.MediaType
		}
		blocks = append(blocks, map[string]any{"type": "image", "source": src})
	}
	if m.Role == "assistant" {
		for _, tc := range m.ToolCalls {
			var input any = map[string]any{}
			if tc.Arguments != "" {
				var parsed any
				if json.Unmarshal([]byte(tc.Arguments), &parsed) == nil {
					input = parsed
				}
			}
			blocks = append(blocks, map[string]any{
				"type":  "tool_use",
				"id":    tc.ID,
				"name":  tc.Name,
				"input": input,
			})
		}
	}
	if blocks == nil {
		// Claude rejects empty content arrays; send an empty text block.
		blocks = []map[string]any{{"type": "text", "text": ""}}
	}
	role := m.Role
	if role == "tool" {
		// Claude tool results ride on a user turn.
		role = "user"
	}
	raw, _ := json.Marshal(blocks)
	return claudeWireMsg{Role: role, Content: raw}
}

// convertToolChoiceToClaude maps the OpenAI tool_choice shapes to Claude's.
func convertToolChoiceToClaude(raw json.RawMessage) json.RawMessage {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		switch s {
		case "auto":
			return mustJSON(map[string]any{"type": "auto"})
		case "none":
			return mustJSON(map[string]any{"type": "none"})
		case "required":
			return mustJSON(map[string]any{"type": "any"})
		}
		return nil
	}
	var obj struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil && obj.Type == "function" && obj.Function.Name != "" {
		return mustJSON(map[string]any{"type": "tool", "name": obj.Function.Name})
	}
	return nil
}

// ---------------------------------------------------------------------------
// Claude response -> IR
// ---------------------------------------------------------------------------

// ParseClaudeChatResponse parses a Claude message response into IR.
func ParseClaudeChatResponse(body []byte) (*ChatResponse, error) {
	var resp struct {
		ID         string            `json:"id"`
		Model      string            `json:"model"`
		Content    []claudeWireBlock `json:"content"`
		StopReason string            `json:"stop_reason"`
		Usage      *struct {
			InputTokens  int64 `json:"input_tokens"`
			OutputTokens int64 `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	ir := &ChatResponse{ID: resp.ID, Model: resp.Model}
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
	for _, b := range resp.Content {
		switch b.Type {
		case "text":
			sb.WriteString(b.Text)
		case "tool_use":
			args := "{}"
			if len(b.Input) > 0 {
				args = string(b.Input)
			}
			ir.ToolCalls = append(ir.ToolCalls, ToolCall{ID: b.ID, Name: b.Name, Arguments: args})
		}
	}
	ir.Text = sb.String()
	return ir, nil
}

// ClaudeStreamEvent is one parsed Claude SSE event.
type ClaudeStreamEvent struct {
	Type string // message_start, content_block_delta, message_delta, message_stop, ...
	// Filled per type:
	Message    *claudeStreamMessage `json:"message,omitempty"`
	TextDelta  string               // content_block_delta.delta.text
	StopReason string               // message_delta.delta.stop_reason
	Usage      *Usage               // message_start.message.usage / message_delta.usage
}

type claudeStreamMessage struct {
	ID         string `json:"id"`
	Model      string `json:"model"`
	StopReason string `json:"stop_reason,omitempty"`
}

// ParseClaudeStreamEvent parses one Claude SSE data payload.
func ParseClaudeStreamEvent(payload []byte) (*ClaudeStreamEvent, error) {
	var ev struct {
		Type    string `json:"type"`
		Message *struct {
			ID    string `json:"id"`
			Model string `json:"model"`
			Usage *struct {
				InputTokens  int64 `json:"input_tokens"`
				OutputTokens int64 `json:"output_tokens"`
			} `json:"usage"`
		} `json:"message"`
		Delta *struct {
			Type       string `json:"type"`
			Text       string `json:"text"`
			StopReason string `json:"stop_reason"`
		} `json:"delta"`
		Usage *struct {
			InputTokens  int64 `json:"input_tokens"`
			OutputTokens int64 `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(payload, &ev); err != nil {
		return nil, err
	}
	out := &ClaudeStreamEvent{Type: ev.Type}
	switch ev.Type {
	case "message_start":
		if ev.Message != nil {
			out.Message = &claudeStreamMessage{ID: ev.Message.ID, Model: ev.Message.Model}
			if ev.Message.Usage != nil {
				out.Usage = &Usage{
					PromptTokens:     ev.Message.Usage.InputTokens,
					CompletionTokens: ev.Message.Usage.OutputTokens,
				}
			}
		}
	case "content_block_delta":
		if ev.Delta != nil {
			out.TextDelta = ev.Delta.Text
		}
	case "message_delta":
		if ev.Delta != nil {
			out.StopReason = ev.Delta.StopReason
		}
		if ev.Usage != nil {
			out.Usage = &Usage{
				PromptTokens:     ev.Usage.InputTokens,
				CompletionTokens: ev.Usage.OutputTokens,
			}
		}
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// IR -> Claude wire (serving claude-in clients)
// ---------------------------------------------------------------------------

// BuildClaudeChatResponse serializes IR into a Claude message response.
func BuildClaudeChatResponse(ir *ChatResponse) ([]byte, error) {
	var out struct {
		ID         string           `json:"id"`
		Type       string           `json:"type"`
		Role       string           `json:"role"`
		Model      string           `json:"model"`
		Content    []map[string]any `json:"content"`
		StopReason string           `json:"stop_reason"`
		Usage      *claudeUsage     `json:"usage,omitempty"`
	}
	out.ID = orDefault(ir.ID, "msg_starhub")
	out.Type = "message"
	out.Role = "assistant"
	out.Model = ir.Model
	out.StopReason = claudeStopReason(ir.FinishReason)
	if ir.Text != "" {
		out.Content = append(out.Content, map[string]any{"type": "text", "text": ir.Text})
	}
	for _, tc := range ir.ToolCalls {
		var input any = map[string]any{}
		if tc.Arguments != "" {
			var parsed any
			if json.Unmarshal([]byte(tc.Arguments), &parsed) == nil {
				input = parsed
			}
		}
		out.Content = append(out.Content, map[string]any{
			"type":  "tool_use",
			"id":    orDefault(tc.ID, "toolu_starhub"),
			"name":  tc.Name,
			"input": input,
		})
	}
	if out.Content == nil {
		out.Content = []map[string]any{{"type": "text", "text": ""}}
	}
	if ir.Usage != nil {
		out.Usage = &claudeUsage{
			InputTokens:  ir.Usage.PromptTokens,
			OutputTokens: ir.Usage.CompletionTokens,
		}
	}
	return json.Marshal(out)
}

type claudeUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

func claudeStopReason(finish string) string {
	switch finish {
	case "stop", "":
		return "end_turn"
	case "length":
		return "max_tokens"
	case "tool_calls":
		return "tool_use"
	}
	return finish
}

func deref(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

func mustJSON(v any) json.RawMessage {
	raw, _ := json.Marshal(v)
	return raw
}

func orEmptySchema(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{"type":"object"}`)
	}
	return raw
}

func orDefault(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

package dto

import (
	"encoding/json"
	"fmt"
	"strings"
)

// openAIWireMessage mirrors the OpenAI message shape (content is either a
// string or an array of typed parts).
type openAIWireMessage struct {
	Role       string               `json:"role"`
	Content    json.RawMessage      `json:"content"`
	Name       string               `json:"name,omitempty"`
	ToolCalls  []openAIWireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string               `json:"tool_call_id,omitempty"`
}

type openAIWireToolCall struct {
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function"`
}

// openAIWireTool is the OpenAI tool definition shape.
type openAIWireTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description,omitempty"`
		Parameters  json.RawMessage `json:"parameters,omitempty"`
	} `json:"function"`
}

// ParseOpenAIChatRequest parses an OpenAI chat completion body into IR.
// Unknown fields are ignored (they only survive the passthrough path).
func ParseOpenAIChatRequest(body []byte) (*ChatRequest, error) {
	var req struct {
		Model               string              `json:"model"`
		Messages            []openAIWireMessage `json:"messages"`
		MaxTokens           *int64              `json:"max_tokens"`
		MaxCompletionTokens *int64              `json:"max_completion_tokens"`
		Temperature         *float64            `json:"temperature"`
		TopP                *float64            `json:"top_p"`
		Stop                json.RawMessage     `json:"stop"`
		Stream              bool                `json:"stream"`
		Tools               []openAIWireTool    `json:"tools"`
		ToolChoice          json.RawMessage     `json:"tool_choice"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	ir := &ChatRequest{
		Model:       req.Model,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stream:      req.Stream,
		ToolChoice:  req.ToolChoice,
	}
	if req.MaxTokens != nil {
		ir.MaxTokens = *req.MaxTokens
	} else if req.MaxCompletionTokens != nil {
		ir.MaxTokens = *req.MaxCompletionTokens
	}
	ir.Stop = parseStopSequences(req.Stop)
	for _, t := range req.Tools {
		if t.Function.Name != "" {
			ir.Tools = append(ir.Tools, Tool{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				Parameters:  t.Function.Parameters,
			})
		}
	}
	for _, m := range req.Messages {
		msg, err := parseOpenAIMessage(m)
		if err != nil {
			return nil, err
		}
		ir.Messages = append(ir.Messages, msg)
	}
	return ir, nil
}

func parseOpenAIMessage(m openAIWireMessage) (ChatMessage, error) {
	msg := ChatMessage{Role: m.Role, ToolCallID: m.ToolCallID}
	for _, tc := range m.ToolCalls {
		msg.ToolCalls = append(msg.ToolCalls, ToolCall{
			ID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments,
		})
	}
	if len(m.Content) == 0 {
		return msg, nil
	}
	var text string
	if err := json.Unmarshal(m.Content, &text); err == nil {
		msg.Text = text
		return msg, nil
	}
	var parts []struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		ImageURL *struct {
			URL string `json:"url"`
		} `json:"image_url"`
	}
	if err := json.Unmarshal(m.Content, &parts); err != nil {
		return msg, fmt.Errorf("message %q has an invalid content field", m.Role)
	}
	var sb strings.Builder
	for _, p := range parts {
		switch p.Type {
		case "text", "input_text", "output_text":
			if p.Text != "" {
				sb.WriteString(p.Text)
				sb.WriteByte(' ')
			}
		case "image_url":
			if p.ImageURL != nil {
				msg.Images = append(msg.Images, ParseDataURLImage(p.ImageURL.URL))
			}
		}
	}
	msg.Text = strings.TrimSpace(sb.String())
	return msg, nil
}

// parseStopSequences accepts "stop": "x", ["x","y"] and null.
func parseStopSequences(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var one string
	if err := json.Unmarshal(raw, &one); err == nil {
		return []string{one}
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err == nil {
		return many
	}
	return nil
}

// ParseDataURLImage splits an OpenAI image_url value (https URL or a
// data:<media>;base64,<payload> reference) into the IR image form.
func ParseDataURLImage(url string) ImageInput {
	if strings.HasPrefix(url, "data:") {
		rest := strings.TrimPrefix(url, "data:")
		media, payload, ok := strings.Cut(rest, ",")
		if ok {
			media = strings.TrimSuffix(media, ";base64")
			return ImageInput{Base64: payload, MediaType: media}
		}
		return ImageInput{Base64: rest}
	}
	return ImageInput{URL: url}
}

// BuildOpenAIChatRequest serializes IR into an OpenAI chat completion body.
func BuildOpenAIChatRequest(ir *ChatRequest) ([]byte, error) {
	var out struct {
		Model       string           `json:"model"`
		Messages    []map[string]any `json:"messages"`
		MaxTokens   *int64           `json:"max_tokens,omitempty"`
		Temperature *float64         `json:"temperature,omitempty"`
		TopP        *float64         `json:"top_p,omitempty"`
		Stream      bool             `json:"stream,omitempty"`
		Tools       []openAIWireTool `json:"tools,omitempty"`
		ToolChoice  json.RawMessage  `json:"tool_choice,omitempty"`
	}
	out.Model = ir.Model
	out.MaxTokens = nonZeroPtr(ir.MaxTokens)
	out.Temperature = ir.Temperature
	out.TopP = ir.TopP
	out.Stream = ir.Stream
	out.ToolChoice = ir.ToolChoice
	for _, m := range ir.Messages {
		out.Messages = append(out.Messages, buildOpenAIMessage(m))
	}
	for _, t := range ir.Tools {
		var tool openAIWireTool
		tool.Type = "function"
		tool.Function.Name = t.Name
		tool.Function.Description = t.Description
		tool.Function.Parameters = t.Parameters
		out.Tools = append(out.Tools, tool)
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	// json omitempty cannot express a non-nil empty stop array; patch it in.
	if len(ir.Stop) > 0 {
		return patchJSONField(raw, "stop", ir.Stop)
	}
	return raw, nil
}

func buildOpenAIMessage(m ChatMessage) map[string]any {
	out := map[string]any{"role": m.Role}
	switch {
	case len(m.Images) > 0:
		parts := make([]map[string]any, 0, len(m.Images)+1)
		if m.Text != "" {
			parts = append(parts, map[string]any{"type": "text", "text": m.Text})
		}
		for _, img := range m.Images {
			parts = append(parts, map[string]any{
				"type":      "image_url",
				"image_url": map[string]any{"url": openAIImageURL(img)},
			})
		}
		out["content"] = parts
	case m.Text != "" || len(m.ToolCalls) == 0:
		out["content"] = m.Text
	}
	if len(m.ToolCalls) > 0 {
		calls := make([]map[string]any, 0, len(m.ToolCalls))
		for _, tc := range m.ToolCalls {
			calls = append(calls, map[string]any{
				"id":   tc.ID,
				"type": "function",
				"function": map[string]any{
					"name":      tc.Name,
					"arguments": tc.Arguments,
				},
			})
		}
		out["tool_calls"] = calls
	}
	if m.ToolCallID != "" {
		out["tool_call_id"] = m.ToolCallID
	}
	return out
}

func openAIImageURL(img ImageInput) string {
	if img.URL != "" {
		return img.URL
	}
	if img.MediaType != "" {
		return "data:" + img.MediaType + ";base64," + img.Base64
	}
	return "data:image/png;base64," + img.Base64
}

// ParseOpenAIChatResponse parses an upstream OpenAI chat completion JSON
// into IR (first choice only; n>1 passthrough is served by the native path).
func ParseOpenAIChatResponse(body []byte) (*ChatResponse, error) {
	var resp struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Choices []struct {
			Message *openAIWireMessage `json:"message"`
			Delta   *openAIWireMessage `json:"delta"`
			Finish  *string            `json:"finish_reason"`
		} `json:"choices"`
		Usage *Usage `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	ir := &ChatResponse{ID: resp.ID, Model: resp.Model, Usage: resp.Usage}
	if len(resp.Choices) > 0 {
		c := resp.Choices[0]
		if c.Finish != nil {
			ir.FinishReason = *c.Finish
		}
		src := c.Message
		if src == nil {
			src = c.Delta
		}
		if src != nil {
			msg, err := parseOpenAIMessage(*src)
			if err != nil {
				return nil, err
			}
			ir.Text = msg.Text
			ir.ToolCalls = msg.ToolCalls
		}
	}
	if ir.FinishReason == "" && len(ir.ToolCalls) > 0 {
		ir.FinishReason = "tool_calls"
	}
	return ir, nil
}

// ParseOpenAIStreamPayload extracts the billing-relevant fields of one
// OpenAI SSE chunk payload.
func ParseOpenAIStreamPayload(payload []byte) (textDelta string, usage *Usage, finishReason string, err error) {
	var chunk struct {
		Choices []struct {
			Delta struct {
				Content json.RawMessage `json:"content"`
			} `json:"delta"`
			Finish *string `json:"finish_reason"`
		} `json:"choices"`
		Usage *Usage `json:"usage"`
	}
	if err := json.Unmarshal(payload, &chunk); err != nil {
		return "", nil, "", err
	}
	if len(chunk.Choices) > 0 {
		textDelta = ContentText(chunk.Choices[0].Delta.Content)
		if chunk.Choices[0].Finish != nil {
			finishReason = *chunk.Choices[0].Finish
		}
	}
	return textDelta, chunk.Usage, finishReason, nil
}

// ContentText extracts the text of an OpenAI message content field (string
// or array of typed parts).
func ContentText(raw json.RawMessage) string {
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

func nonZeroPtr(v int64) *int64 {
	if v == 0 {
		return nil
	}
	return &v
}

// patchJSONField injects or replaces a top-level field of a JSON object
// (used where struct tags cannot express the desired presence rules).
func patchJSONField(raw []byte, field string, value any) ([]byte, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	m[field] = encoded
	return json.Marshal(m)
}

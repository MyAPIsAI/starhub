package dto

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// InboundFormat identifies the protocol the client speaks. The chat relay
// parses any inbound format into the IR below and re-serializes responses
// (and error envelopes) back into the same format.
type InboundFormat string

const (
	InboundOpenAI InboundFormat = "openai"
	InboundClaude InboundFormat = "claude"
	InboundGemini InboundFormat = "gemini"
)

// ImageInput is an image attachment of a chat message, in either URL or
// base64 form (one of the two is set).
type ImageInput struct {
	URL       string // http(s) reference
	Base64    string // raw base64 payload (no data: prefix)
	MediaType string // e.g. "image/png"; empty means infer from URL
}

// ToolCall is one assistant tool invocation. Arguments stays a JSON string
// (the OpenAI wire format) so round-trips never reserialize user data.
type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

// Tool is a function/tool definition (converted to each provider's shape).
type Tool struct {
	Name        string
	Description string
	Parameters  json.RawMessage // JSON schema, passed through verbatim
}

// ChatMessage is the IR message. Roles: system, user, assistant, tool.
// A tool result carries ToolCallID (OpenAI tool_call_id / Claude
// tool_use_id); an assistant tool call carries ToolCalls.
type ChatMessage struct {
	Role       string
	Text       string
	Images     []ImageInput
	ToolCalls  []ToolCall
	ToolCallID string
}

// ChatRequest is the IR of a chat completion request. Every inbound protocol
// parses into it and every channel adaptor serializes from it.
type ChatRequest struct {
	Model       string
	Messages    []ChatMessage
	MaxTokens   int64 // max_tokens / max_completion_tokens / Claude max_tokens
	Temperature *float64
	TopP        *float64
	Stop        []string
	Stream      bool
	Tools       []Tool
	ToolChoice  json.RawMessage // passed through; converters map the common shapes
}

// ChatResponse is the IR of a non-stream chat completion (single choice).
type ChatResponse struct {
	ID           string
	Model        string
	Text         string
	ToolCalls    []ToolCall
	FinishReason string // "stop", "length", "tool_calls", ...
	Usage        *Usage
}

// ChatStreamEvent is one IR streaming event pulled from an upstream SSE
// stream. TextDelta accumulates into the assistant message; Usage snapshots
// the latest usage report (the final snapshot settles the bill).
type ChatStreamEvent struct {
	TextDelta    string
	FinishReason string
	Usage        *Usage
}

// PromptText concatenates the text of all messages (the billing estimate
// input for the pre-charge).
func (r *ChatRequest) PromptText() string {
	if r == nil {
		return ""
	}
	parts := make([]string, 0, len(r.Messages))
	for _, m := range r.Messages {
		if m.Text != "" {
			parts = append(parts, m.Text)
		}
	}
	return strings.Join(parts, " ")
}

// EstimateTokens is the char-based tokenizer approximation used by the
// pre-charge (roughly 4 chars per token).
func EstimateTokens(text string) int64 {
	n := utf8.RuneCountInString(text)
	if n == 0 {
		return 0
	}
	return int64((n + 3) / 4)
}

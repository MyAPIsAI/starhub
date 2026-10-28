package dto

import (
	"bytes"
	"encoding/json"
	"errors"
)

// ResponsesRequest describes routing and reservation inputs only. The relay
// keeps the original JSON for native forwarding, including extension fields.
type ResponsesRequest struct {
	Model              string            `json:"model"`
	Input              json.RawMessage   `json:"input"`
	Instructions       string            `json:"instructions"`
	Tools              []json.RawMessage `json:"tools"`
	MaxOutputTokens    *int64            `json:"max_output_tokens"`
	Stream             bool              `json:"stream"`
	Background         bool              `json:"background"`
	PreviousResponseID string            `json:"previous_response_id"`
	Conversation       json.RawMessage   `json:"conversation"`
	Reasoning          json.RawMessage   `json:"reasoning"`
	InputTokens        int64             `json:"-"`
	FirstUser          json.RawMessage   `json:"-"`
	HasImages          bool              `json:"-"`
	HasTools           bool              `json:"-"`
	HasReasoning       bool              `json:"-"`
	HasFiles           bool              `json:"-"`
	Continuation       bool              `json:"-"`
}

func ParseResponsesRequest(body []byte) (*ResponsesRequest, error) {
	if trimmed := bytes.TrimSpace(body); len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, errors.New("Responses request must be a JSON object")
	}
	var req ResponsesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	if req.Model == "" {
		return nil, errors.New("Responses model is required")
	}
	if req.MaxOutputTokens != nil && *req.MaxOutputTokens <= 0 {
		return nil, errors.New("max_output_tokens must be positive")
	}
	req.InputTokens = EstimateTokens(req.Instructions)
	req.HasTools = len(req.Tools) > 0
	req.HasReasoning = len(req.Reasoning) > 0 && !bytes.Equal(bytes.TrimSpace(req.Reasoning), []byte("null"))
	for _, tool := range req.Tools {
		req.InputTokens += EstimateTokens(string(tool))
	}
	input := bytes.TrimSpace(req.Input)
	if len(input) == 0 {
		return &req, nil
	}
	if input[0] == '"' {
		var text string
		if err := json.Unmarshal(input, &text); err != nil {
			return nil, err
		}
		req.InputTokens += EstimateTokens(text)
		req.FirstUser = append(json.RawMessage(nil), input...)
		return &req, nil
	}
	var items []json.RawMessage
	if input[0] != '[' {
		return nil, errors.New("Responses input must be a string or an array")
	}
	if err := json.Unmarshal(input, &items); err != nil {
		return nil, err
	}
	for _, raw := range items {
		if err := req.readInputItem(raw); err != nil {
			return nil, err
		}
	}
	return &req, nil
}

func (req *ResponsesRequest) readInputItem(raw json.RawMessage) error {
	var item struct {
		Type             string          `json:"type"`
		Role             string          `json:"role"`
		Content          json.RawMessage `json:"content"`
		Name             string          `json:"name"`
		CallID           string          `json:"call_id"`
		Arguments        string          `json:"arguments"`
		Input            string          `json:"input"`
		Output           json.RawMessage `json:"output"`
		Summary          json.RawMessage `json:"summary"`
		EncryptedContent string          `json:"encrypted_content"`
	}
	if err := json.Unmarshal(raw, &item); err != nil {
		return err
	}
	switch item.Type {
	case "", "message":
		switch item.Role {
		case "user", "assistant", "system", "developer":
		default:
			return errors.New("invalid Responses message role")
		}
		if item.Role == "user" && req.FirstUser == nil && !req.Continuation {
			req.FirstUser = append(json.RawMessage(nil), item.Content...)
		}
		return req.readContent(item.Content)
	case "function_call", "custom_tool_call":
		req.HasTools, req.Continuation = true, true
		req.InputTokens += EstimateTokens(item.Name) + EstimateTokens(item.CallID) + EstimateTokens(item.Arguments) + EstimateTokens(item.Input)
	case "function_call_output", "custom_tool_call_output":
		req.HasTools, req.Continuation = true, true
		req.InputTokens += EstimateTokens(item.CallID)
		return req.readContent(item.Output)
	case "reasoning", "compaction":
		req.HasReasoning, req.Continuation = true, true
		// Opaque state has no local tokenizer. Its encoded size contributes
		// a byte-based estimate instead of being treated as free context.
		req.InputTokens += EstimateTokens(item.EncryptedContent)
		if len(item.Summary) > 0 && !bytes.Equal(bytes.TrimSpace(item.Summary), []byte("null")) {
			if err := req.readContent(item.Summary); err != nil {
				return err
			}
		}
		if len(item.Content) > 0 && !bytes.Equal(bytes.TrimSpace(item.Content), []byte("null")) {
			return req.readContent(item.Content)
		}
	default:
		return errors.New("unsupported Responses input item type; send complete message and tool history")
	}
	return nil
}

func (req *ResponsesRequest) readContent(raw json.RawMessage) error {
	content := bytes.TrimSpace(raw)
	if len(content) == 0 {
		return errors.New("Responses content is required")
	}
	if content[0] == '"' {
		var text string
		if err := json.Unmarshal(content, &text); err != nil {
			return err
		}
		req.InputTokens += EstimateTokens(text)
		return nil
	}
	if content[0] != '[' {
		return errors.New("Responses content must be a string or an array")
	}
	var parts []struct {
		Type    string `json:"type"`
		Text    string `json:"text"`
		Refusal string `json:"refusal"`
		FileID  string `json:"file_id"`
	}
	if err := json.Unmarshal(content, &parts); err != nil {
		return err
	}
	for _, part := range parts {
		switch part.Type {
		case "input_text", "output_text", "text", "summary_text", "reasoning_text":
			req.InputTokens += EstimateTokens(part.Text)
		case "refusal":
			req.InputTokens += EstimateTokens(part.Refusal)
		case "input_image":
			req.HasImages = true
			req.HasFiles = req.HasFiles || part.FileID != ""
		case "input_file":
			req.HasFiles = true
		default:
			return errors.New("unsupported Responses content type")
		}
	}
	return nil
}

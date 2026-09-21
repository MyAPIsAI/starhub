package dto

import (
	"encoding/json"
	"strings"
)

// ---------------------------------------------------------------------------
// IR -> Gemini (gemini channels)
// ---------------------------------------------------------------------------

type geminiPart struct {
	Text         string            `json:"text,omitempty"`
	InlineData   *geminiInlineData `json:"inline_data,omitempty"`
	FileData     *geminiFileData   `json:"file_data,omitempty"`
	FunctionCall *struct {
		Name string          `json:"name"`
		Args json.RawMessage `json:"args"`
	} `json:"functionCall,omitempty"`
	FunctionResponse *struct {
		Name     string          `json:"name"`
		Response json.RawMessage `json:"response"`
	} `json:"functionResponse,omitempty"`
}

type geminiInlineData struct {
	MimeType string `json:"mime_type"`
	Data     string `json:"data"`
}

type geminiFileData struct {
	MimeType string `json:"mime_type,omitempty"`
	FileURI  string `json:"file_uri"`
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

// BuildGeminiChatRequest serializes IR into a Gemini generateContent body.
func BuildGeminiChatRequest(ir *ChatRequest) ([]byte, error) {
	var out struct {
		SystemInstruction *geminiContent   `json:"systemInstruction,omitempty"`
		Contents          []geminiContent  `json:"contents"`
		GenerationConfig  map[string]any   `json:"generationConfig,omitempty"`
		Tools             []map[string]any `json:"tools,omitempty"`
		ToolConfig        map[string]any   `json:"toolConfig,omitempty"`
	}
	for _, m := range ir.Messages {
		switch m.Role {
		case "system":
			if out.SystemInstruction == nil {
				out.SystemInstruction = &geminiContent{
					Parts: []geminiPart{{Text: m.Text}},
				}
			}
		case "user", "assistant":
			content := geminiContent{Parts: []geminiPart{}}
			content.Role = geminiRole(m.Role)
			if m.Text != "" {
				content.Parts = append(content.Parts, geminiPart{Text: m.Text})
			}
			for _, img := range m.Images {
				if img.URL == "" && img.Base64 != "" {
					mime := img.MediaType
					if mime == "" {
						mime = "image/png"
					}
					content.Parts = append(content.Parts, geminiPart{
						InlineData: &geminiInlineData{MimeType: mime, Data: img.Base64},
					})
				}
			}
			for _, tc := range m.ToolCalls {
				call := &struct {
					Name string          `json:"name"`
					Args json.RawMessage `json:"args"`
				}{Name: tc.Name}
				if tc.Arguments != "" {
					var parsed any
					if json.Unmarshal([]byte(tc.Arguments), &parsed) == nil {
						call.Args = mustJSON(parsed)
					}
				}
				if call.Args == nil {
					call.Args = json.RawMessage(`{}`)
				}
				content.Parts = append(content.Parts, geminiPart{FunctionCall: call})
			}
			if len(content.Parts) == 0 {
				content.Parts = append(content.Parts, geminiPart{Text: " "})
			}
			out.Contents = append(out.Contents, content)
		case "tool":
			resp := &struct {
				Name     string          `json:"name"`
				Response json.RawMessage `json:"response"`
			}{Name: m.ToolCallID}
			if resp.Name == "" {
				resp.Name = m.Text
			}
			resp.Response = mustJSON(map[string]any{"result": m.Text})
			out.Contents = append(out.Contents, geminiContent{
				Role:  "user",
				Parts: []geminiPart{{FunctionResponse: resp}},
			})
		}
	}
	if len(ir.Tools) > 0 {
		decs := make([]map[string]any, 0, len(ir.Tools))
		for _, t := range ir.Tools {
			dec := map[string]any{"name": t.Name}
			if t.Description != "" {
				dec["description"] = t.Description
			}
			if len(t.Parameters) > 0 {
				dec["parameters"] = t.Parameters
			}
			decs = append(decs, dec)
		}
		out.Tools = append(out.Tools, map[string]any{"function_declarations": decs})
	}
	cfg := map[string]any{}
	if ir.MaxTokens > 0 {
		cfg["maxOutputTokens"] = ir.MaxTokens
	}
	if ir.Temperature != nil {
		cfg["temperature"] = *ir.Temperature
	}
	if ir.TopP != nil {
		cfg["topP"] = *ir.TopP
	}
	if len(ir.Stop) > 0 {
		cfg["stopSequences"] = ir.Stop
	}
	if len(cfg) > 0 {
		out.GenerationConfig = cfg
	}
	return json.Marshal(out)
}

func geminiRole(role string) string {
	if role == "assistant" {
		return "model"
	}
	return "user"
}

// ---------------------------------------------------------------------------
// Gemini response -> IR
// ---------------------------------------------------------------------------

// ParseGeminiChatResponse parses a generateContent response into IR.
func ParseGeminiChatResponse(body []byte) (*ChatResponse, error) {
	var resp struct {
		Candidates []struct {
			Content      geminiContent `json:"content"`
			FinishReason string        `json:"finishReason"`
		} `json:"candidates"`
		UsageMetadata *struct {
			PromptTokenCount     int64 `json:"promptTokenCount"`
			CandidatesTokenCount int64 `json:"candidatesTokenCount"`
			TotalTokenCount      int64 `json:"totalTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	ir := &ChatResponse{}
	if resp.UsageMetadata != nil {
		ir.Usage = &Usage{
			PromptTokens:     resp.UsageMetadata.PromptTokenCount,
			CompletionTokens: resp.UsageMetadata.CandidatesTokenCount,
		}
	}
	if len(resp.Candidates) == 0 {
		return ir, nil
	}
	c := resp.Candidates[0]
	switch c.FinishReason {
	case "STOP":
		ir.FinishReason = "stop"
	case "MAX_TOKENS":
		ir.FinishReason = "length"
	default:
		ir.FinishReason = c.FinishReason
	}
	var sb strings.Builder
	for _, p := range c.Content.Parts {
		if p.Text != "" {
			sb.WriteString(p.Text)
		}
		if p.FunctionCall != nil {
			args := "{}"
			if len(p.FunctionCall.Args) > 0 {
				args = string(p.FunctionCall.Args)
			}
			ir.ToolCalls = append(ir.ToolCalls, ToolCall{
				Name: p.FunctionCall.Name, Arguments: args,
			})
		}
	}
	ir.Text = sb.String()
	if ir.FinishReason == "" && len(ir.ToolCalls) > 0 {
		ir.FinishReason = "tool_calls"
	}
	return ir, nil
}

// ParseGeminiStreamPayload extracts the IR delta of one streamGenerateContent
// SSE chunk. Later chunks carry the final usageMetadata.
func ParseGeminiStreamPayload(payload []byte) (*ChatStreamEvent, error) {
	resp, err := ParseGeminiChatResponse(payload)
	if err != nil {
		return nil, err
	}
	ev := &ChatStreamEvent{
		TextDelta:    resp.Text,
		FinishReason: resp.FinishReason,
		Usage:        resp.Usage,
	}
	if ev.Usage != nil && ev.Usage.PromptTokens == 0 && ev.Usage.CompletionTokens == 0 {
		ev.Usage = nil
	}
	return ev, nil
}

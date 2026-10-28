package dto

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

var geminiModelID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

// ParseGeminiPath reads a public native method without accepting arbitrary
// upstream paths, resource references or query strings as a model ID.
func ParseGeminiPath(path string) (model string, stream bool, err error) {
	method, ok := strings.CutPrefix(path, "/v1beta/models/")
	if !ok {
		return "", false, errors.New("invalid native Gemini path")
	}
	model, action, ok := strings.Cut(method, ":")
	if !ok || !geminiModelID.MatchString(model) {
		return "", false, errors.New("invalid native Gemini model identifier")
	}
	switch action {
	case "generateContent":
		return model, false, nil
	case "streamGenerateContent":
		return model, true, nil
	default:
		return "", false, errors.New("unsupported native Gemini method")
	}
}

// GeminiRequest describes routing and reservation inputs. Native forwarding
// uses the original body so signatures, tool IDs and extension data survive.
type GeminiRequest struct {
	Contents          []geminiRequestContent `json:"contents"`
	SystemInstruction *geminiRequestContent  `json:"systemInstruction"`
	Tools             []json.RawMessage      `json:"tools"`
	CachedContent     string                 `json:"cachedContent"`
	ContinuationToken string                 `json:"continuationToken"`
	GenerationConfig  struct {
		MaxOutputTokens    *int64          `json:"maxOutputTokens"`
		CandidateCount     *int64          `json:"candidateCount"`
		ThinkingConfig     json.RawMessage `json:"thinkingConfig"`
		ResponseModalities []string        `json:"responseModalities"`
	} `json:"generationConfig"`
	InputTokens  int64           `json:"-"`
	FirstUser    json.RawMessage `json:"-"`
	HasImages    bool            `json:"-"`
	HasTools     bool            `json:"-"`
	HasReasoning bool            `json:"-"`
	Continuation bool            `json:"-"`
}

type geminiRequestContent struct {
	Role  string            `json:"role"`
	Parts []json.RawMessage `json:"parts"`
}

func ParseGeminiRequest(body []byte) (*GeminiRequest, error) {
	if trimmed := bytes.TrimSpace(body); len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, errors.New("Gemini request must be a JSON object")
	}
	var req GeminiRequest
	canonical, err := canonicalGeminiFields(body, map[string]string{"system_instruction": "systemInstruction", "generation_config": "generationConfig", "cached_content": "cachedContent", "continuation_token": "continuationToken"})
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(canonical, &fields); err != nil {
		return nil, err
	}
	if config := fields["generationConfig"]; len(config) > 0 && !bytes.Equal(bytes.TrimSpace(config), []byte("null")) {
		fields["generationConfig"], err = canonicalGeminiFields(config, map[string]string{"max_output_tokens": "maxOutputTokens", "candidate_count": "candidateCount", "thinking_config": "thinkingConfig", "response_modalities": "responseModalities"})
		if err != nil {
			return nil, err
		}
		canonical, _ = json.Marshal(fields)
	}
	if err := json.Unmarshal(canonical, &req); err != nil {
		return nil, err
	}
	if len(req.Contents) == 0 {
		return nil, errors.New("Gemini contents are required")
	}
	if req.CachedContent != "" || req.ContinuationToken != "" {
		return nil, errors.New("provider-owned Gemini references are not available yet; send complete contents")
	}
	if limit := req.GenerationConfig.MaxOutputTokens; limit != nil && *limit <= 0 {
		return nil, errors.New("maxOutputTokens must be positive")
	}
	if count := req.GenerationConfig.CandidateCount; count != nil && *count != 1 {
		return nil, errors.New("native Gemini currently supports candidateCount = 1")
	}
	for _, modality := range req.GenerationConfig.ResponseModalities {
		if modality != "TEXT" {
			return nil, errors.New("native Gemini currently supports TEXT output only")
		}
	}
	req.HasReasoning = len(req.GenerationConfig.ThinkingConfig) > 0 && !bytes.Equal(bytes.TrimSpace(req.GenerationConfig.ThinkingConfig), []byte("null"))
	for _, raw := range req.Tools {
		var tool map[string]json.RawMessage
		if err := json.Unmarshal(raw, &tool); err != nil {
			return nil, err
		}
		if len(tool) == 0 {
			return nil, errors.New("empty Gemini tool")
		}
		for key := range tool {
			if key != "functionDeclarations" && key != "function_declarations" {
				return nil, errors.New("native Gemini currently supports function declarations only")
			}
		}
		req.HasTools = true
		req.InputTokens += EstimateTokens(string(raw))
	}
	if req.SystemInstruction != nil {
		if err := req.readParts(req.SystemInstruction.Parts); err != nil {
			return nil, err
		}
	}
	for _, content := range req.Contents {
		if content.Role != "" && content.Role != "user" && content.Role != "model" {
			return nil, errors.New("invalid Gemini content role")
		}
		first := req.FirstUser == nil && !req.Continuation && content.Role != "model"
		if err := req.readParts(content.Parts); err != nil {
			return nil, err
		}
		if first && !req.Continuation {
			req.FirstUser, _ = json.Marshal(content.Parts)
		}
	}
	return &req, nil
}

func (req *GeminiRequest) readParts(parts []json.RawMessage) error {
	return req.readPartsDepth(parts, 0)
}

func (req *GeminiRequest) readPartsDepth(parts []json.RawMessage, depth int) error {
	if depth > 8 {
		return errors.New("Gemini function response nesting exceeds the limit")
	}
	if len(parts) == 0 {
		return errors.New("Gemini content parts are required")
	}
	for _, raw := range parts {
		raw, err := canonicalGeminiFields(raw, map[string]string{"thought_signature": "thoughtSignature", "function_call": "functionCall", "function_response": "functionResponse", "inline_data": "inlineData", "file_data": "fileData", "executable_code": "executableCode", "code_execution_result": "codeExecutionResult"})
		if err != nil {
			return err
		}
		var part map[string]json.RawMessage
		if err := json.Unmarshal(raw, &part); err != nil {
			return err
		}
		known := false
		for key, value := range part {
			switch key {
			case "text", "thoughtSignature", "thought_signature":
				var text string
				if err := json.Unmarshal(value, &text); err != nil {
					return err
				}
				req.InputTokens += EstimateTokens(text)
				if key == "text" {
					known = true
				} else {
					req.HasReasoning = true
				}
			case "functionCall", "function_call", "functionResponse", "function_response":
				var function map[string]json.RawMessage
				if err := json.Unmarshal(value, &function); err != nil || len(function) == 0 {
					return errors.New("invalid Gemini function part")
				}
				if rawParts := function["parts"]; len(rawParts) > 0 {
					var nested []json.RawMessage
					if err := json.Unmarshal(rawParts, &nested); err != nil {
						return err
					}
					if err := req.readPartsDepth(nested, depth+1); err != nil {
						return err
					}
					delete(function, "parts")
					value, _ = json.Marshal(function)
				}
				req.InputTokens += EstimateTokens(string(value))
				req.HasTools, req.Continuation, known = true, true, true
			case "inlineData", "inline_data":
				var media struct {
					MimeType      string `json:"mimeType"`
					SnakeMimeType string `json:"mime_type"`
					Data          string `json:"data"`
				}
				if err := json.Unmarshal(value, &media); err != nil {
					return err
				}
				if media.MimeType == "" {
					media.MimeType = media.SnakeMimeType
				}
				if !strings.HasPrefix(media.MimeType, "image/") || media.Data == "" {
					return errors.New("native Gemini inline media currently requires an image MIME type and data")
				}
				req.HasImages, known = true, true
			case "fileData", "file_data", "executableCode", "executable_code", "codeExecutionResult", "code_execution_result":
				return errors.New("Gemini file references and hosted execution are not available yet")
			case "thought":
				var thought bool
				if err := json.Unmarshal(value, &thought); err != nil {
					return err
				}
				req.HasReasoning = req.HasReasoning || thought
			}
		}
		if !known {
			return errors.New("unsupported Gemini part; send text, inline images or function history")
		}
	}
	return nil
}

// Protobuf JSON accepts both lowerCamelCase and original snake_case names.
// Canonicalization is for accounting only; forwarding retains the raw body.
func canonicalGeminiFields(raw []byte, aliases map[string]string) ([]byte, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return nil, errors.New("Gemini field must be a JSON object")
	}
	for alias, name := range aliases {
		if value, ok := fields[alias]; ok {
			if _, exists := fields[name]; exists {
				return nil, errors.New("conflicting Gemini field aliases")
			}
			fields[name] = value
			delete(fields, alias)
		}
	}
	return json.Marshal(fields)
}

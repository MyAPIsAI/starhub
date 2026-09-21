package dto

import (
	"encoding/json"
	"fmt"
	"regexp"
)

// sizePattern matches "<int>x<int>" size values.
var sizePattern = regexp.MustCompile(`^[0-9]+x[0-9]+$`)

// ---------------------------------------------------------------------------
// Audio (TTS / STT)
// ---------------------------------------------------------------------------

// MaxTTSInputChars bounds the TTS input text (the OpenAI limit is 4096);
// the input is the billed quantity, so it must not be unbounded.
const MaxTTSInputChars = 4096

// TTSRequest is a /v1/audio/speech request. Unknown fields survive in Extra.
type TTSRequest struct {
	Model          string                     `json:"model"`
	Input          string                     `json:"input"`
	Voice          string                     `json:"voice,omitempty"`
	ResponseFormat string                     `json:"response_format,omitempty"`
	Speed          *float64                   `json:"speed,omitempty"`
	Extra          map[string]json.RawMessage `json:"-"`
}

// ParseTTSRequest parses and validates a TTS request body.
func ParseTTSRequest(body []byte) (*TTSRequest, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	req := &TTSRequest{Extra: map[string]json.RawMessage{}}
	for k, v := range raw {
		switch k {
		case "model":
			if err := json.Unmarshal(v, &req.Model); err != nil {
				return nil, fmt.Errorf("invalid %s: %w", k, err)
			}
		case "input":
			if err := json.Unmarshal(v, &req.Input); err != nil {
				return nil, fmt.Errorf("invalid %s: %w", k, err)
			}
		case "voice":
			_ = json.Unmarshal(v, &req.Voice)
		case "response_format":
			_ = json.Unmarshal(v, &req.ResponseFormat)
		case "speed":
			_ = json.Unmarshal(v, &req.Speed)
		default:
			req.Extra[k] = v
		}
	}
	if req.Model == "" {
		return nil, fmt.Errorf("the \"model\" field is required")
	}
	if req.Input == "" {
		return nil, fmt.Errorf("the \"input\" field is required")
	}
	if n := len([]rune(req.Input)); n > MaxTTSInputChars {
		return nil, fmt.Errorf("input exceeds the maximum of %d characters, got %d", MaxTTSInputChars, n)
	}
	return req, nil
}

// MarshalBody serializes the TTS request (Extra fields flattened).
func (r *TTSRequest) MarshalBody() ([]byte, error) {
	base, err := json.Marshal(struct {
		Model          string   `json:"model"`
		Input          string   `json:"input"`
		Voice          string   `json:"voice,omitempty"`
		ResponseFormat string   `json:"response_format,omitempty"`
		Speed          *float64 `json:"speed,omitempty"`
	}{r.Model, r.Input, r.Voice, r.ResponseFormat, r.Speed})
	if err != nil {
		return nil, err
	}
	if len(r.Extra) == 0 {
		return base, nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(base, &m); err != nil {
		return nil, err
	}
	for k, v := range r.Extra {
		m[k] = v
	}
	return json.Marshal(m)
}

// TranscriptionResponse is the billing-relevant subset of a /v1/audio/*
// response. Duration (seconds) only arrives with verbose_json and drives
// the audio-duration billing.
type TranscriptionResponse struct {
	Text     string  `json:"text"`
	Duration float64 `json:"duration,omitempty"`
}

// ParseTranscriptionResponse extracts text / duration from an STT response.
func ParseTranscriptionResponse(body []byte) (*TranscriptionResponse, error) {
	var resp TranscriptionResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ---------------------------------------------------------------------------
// Embeddings
// ---------------------------------------------------------------------------

// MaxEmbeddingInputs bounds the number of input items (each item is billed,
// so the multiplier must be capped before the pre-charge).
const MaxEmbeddingInputs = 2048

// EmbeddingRequest is a /v1/embeddings request (input may be a string or an
// array of strings; both parse into Items).
type EmbeddingRequest struct {
	Model          string
	Items          []string
	EncodingFormat string
	Dimensions     *int
	Extra          map[string]json.RawMessage
}

// ParseEmbeddingRequest parses and validates an embeddings body.
func ParseEmbeddingRequest(body []byte) (*EmbeddingRequest, error) {
	var raw struct {
		Model          string          `json:"model"`
		Input          json.RawMessage `json:"input"`
		EncodingFormat string          `json:"encoding_format"`
		Dimensions     *int            `json:"dimensions"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	req := &EmbeddingRequest{
		Model:          raw.Model,
		EncodingFormat: raw.EncodingFormat,
		Dimensions:     raw.Dimensions,
		Extra:          map[string]json.RawMessage{},
	}
	if req.Model == "" {
		return nil, fmt.Errorf("the \"model\" field is required")
	}
	if len(raw.Input) == 0 {
		return nil, fmt.Errorf("the \"input\" field is required")
	}
	var one string
	if err := json.Unmarshal(raw.Input, &one); err == nil {
		req.Items = []string{one}
	} else {
		var many []string
		if err := json.Unmarshal(raw.Input, &many); err != nil {
			return nil, fmt.Errorf("input must be a string or an array of strings")
		}
		req.Items = many
	}
	if len(req.Items) == 0 {
		return nil, fmt.Errorf("input must not be empty")
	}
	if len(req.Items) > MaxEmbeddingInputs {
		return nil, fmt.Errorf("input exceeds the maximum of %d items, got %d", MaxEmbeddingInputs, len(req.Items))
	}
	// preserve unknown fields
	var all map[string]json.RawMessage
	if err := json.Unmarshal(body, &all); err == nil {
		for k, v := range all {
			switch k {
			case "model", "input", "encoding_format", "dimensions":
			default:
				req.Extra[k] = v
			}
		}
	}
	return req, nil
}

// InputText concatenates the input items (the token estimate input).
func (r *EmbeddingRequest) InputText() string {
	if r == nil {
		return ""
	}
	text := ""
	for i, item := range r.Items {
		if i > 0 {
			text += " "
		}
		text += item
	}
	return text
}

// EmbeddingUsage is the embeddings usage block.
type EmbeddingUsage struct {
	PromptTokens int64 `json:"prompt_tokens"`
	TotalTokens  int64 `json:"total_tokens"`
}

// ParseEmbeddingUsage pulls the usage out of an embeddings response.
func ParseEmbeddingUsage(body []byte) (*EmbeddingUsage, error) {
	var resp struct {
		Usage *EmbeddingUsage `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return resp.Usage, nil
}

// ---------------------------------------------------------------------------
// Rerank
// ---------------------------------------------------------------------------

// MaxRerankDocuments bounds the documents array (billed quantity).
const MaxRerankDocuments = 1024

// RerankRequest is a /v1/rerank request (Cohere-compatible shape).
type RerankRequest struct {
	Model           string
	Query           string
	Documents       []string
	TopN            *int
	ReturnDocuments *bool
	Extra           map[string]json.RawMessage
}

// ParseRerankRequest parses and validates a rerank body.
func ParseRerankRequest(body []byte) (*RerankRequest, error) {
	var raw struct {
		Model           string   `json:"model"`
		Query           string   `json:"query"`
		Documents       []string `json:"documents"`
		TopN            *int     `json:"top_n"`
		ReturnDocuments *bool    `json:"return_documents"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	req := &RerankRequest{
		Model:           raw.Model,
		Query:           raw.Query,
		Documents:       raw.Documents,
		TopN:            raw.TopN,
		ReturnDocuments: raw.ReturnDocuments,
		Extra:           map[string]json.RawMessage{},
	}
	if req.Model == "" {
		return nil, fmt.Errorf("the \"model\" field is required")
	}
	if req.Query == "" {
		return nil, fmt.Errorf("the \"query\" field is required")
	}
	if len(req.Documents) == 0 {
		return nil, fmt.Errorf("the \"documents\" field is required")
	}
	if len(req.Documents) > MaxRerankDocuments {
		return nil, fmt.Errorf("documents exceeds the maximum of %d items, got %d", MaxRerankDocuments, len(req.Documents))
	}
	if req.TopN != nil && *req.TopN < 0 {
		return nil, fmt.Errorf("top_n must be >= 0")
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(body, &all); err == nil {
		for k, v := range all {
			switch k {
			case "model", "query", "documents", "top_n", "return_documents":
			default:
				req.Extra[k] = v
			}
		}
	}
	return req, nil
}

// InputText concatenates query + documents for the token estimate.
func (r *RerankRequest) InputText() string {
	if r == nil {
		return ""
	}
	text := r.Query
	for _, doc := range r.Documents {
		text += " " + doc
	}
	return text
}

// RerankUsage is the rerank usage block; providers disagree on the field
// name (input_tokens / billed_tokens / billing_tokens), so every known
// alias is tried.
type RerankUsage struct {
	PromptTokens     int64 `json:"input_tokens"`
	CompletionTokens int64 `json:"output_tokens"`
	SearchUnits      int64 `json:"search_units"`
}

// ParseRerankUsage pulls the usage out of a rerank response.
func ParseRerankUsage(body []byte) (*Usage, error) {
	var resp struct {
		Usage json.RawMessage `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	if len(resp.Usage) == 0 {
		return nil, nil
	}
	var u struct {
		InputTokens   *int64 `json:"input_tokens"`
		BilledTokens  *int64 `json:"billed_tokens"`
		BillingTokens *int64 `json:"billing_tokens"`
		OutputTokens  *int64 `json:"output_tokens"`
		TotalTokens   *int64 `json:"total_tokens"`
	}
	if err := json.Unmarshal(resp.Usage, &u); err != nil {
		return nil, err
	}
	prompt := firstInt64(u.InputTokens, u.BilledTokens, u.BillingTokens, u.TotalTokens)
	return &Usage{
		PromptTokens:     prompt,
		CompletionTokens: derefInt64(u.OutputTokens),
	}, nil
}

func firstInt64(vals ...*int64) int64 {
	for _, v := range vals {
		if v != nil {
			return *v
		}
	}
	return 0
}

func derefInt64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

package dto

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// MaxImageN bounds the "n" of an image request on every parsing path (JSON
// and multipart): an unbounded n (including uint wrap-around values) would
// overflow the image billing into negative territory (P3.1).
const MaxImageN = 128

// ImageRequest is the billing-relevant image generation/edit request.
// Optional scalars are pointers + omitempty; unknown fields land in Extra
// and are re-serialized verbatim so provider-specific parameters survive
// the relay (P3.1 DTO discipline).
type ImageRequest struct {
	Model          string                     `json:"model"`
	Prompt         string                     `json:"prompt"`
	N              *uint                      `json:"n,omitempty"`
	Size           string                     `json:"size,omitempty"`
	Quality        string                     `json:"quality,omitempty"`
	Style          string                     `json:"style,omitempty"`
	ResponseFormat string                     `json:"response_format,omitempty"`
	Stream         *bool                      `json:"stream,omitempty"`
	Watermark      *bool                      `json:"watermark,omitempty"`
	User           string                     `json:"user,omitempty"`
	Extra          map[string]json.RawMessage `json:"-"`
}

// Count returns the requested image count (default 1).
func (r *ImageRequest) Count() uint {
	if r == nil || r.N == nil || *r.N == 0 {
		return 1
	}
	return *r.N
}

// ValidateN checks the n bound. It runs on the JSON path and on the
// multipart path (where n arrives as a string form field) so both entries
// share one billing gate.
func (r *ImageRequest) ValidateN() error {
	if r.N == nil {
		return nil
	}
	if *r.N == 0 || *r.N > MaxImageN {
		return fmt.Errorf("n must be between 1 and %d, got %d", MaxImageN, *r.N)
	}
	return nil
}

// imageModelSizes is the per-model size whitelist of the legacy DALL-E
// models: dall-e-2 (and the bare "dall-e" alias) only generates squares,
// dall-e-3 adds the tall/wide variants. Other models (gpt-image-1, provider
// specifics) accept any <width>x<height> shape and "auto".
var imageModelSizes = map[string][]string{
	"dall-e-2": {"256x256", "512x512", "1024x1024"},
	"dall-e":   {"256x256", "512x512", "1024x1024"},
	"dall-e-3": {"1024x1024", "1024x1792", "1792x1024"},
}

// ValidateSize rejects sizes the gateway cannot price. The full-width
// multiplication sign is a classic copy-paste artifact and gets its own
// error hint (P3.1).
func (r *ImageRequest) ValidateSize() error {
	if strings.Contains(r.Size, "×") {
		return errors.New("size contains the full-width character \"×\"; use \"x\" (e.g. 1024x1024)")
	}
	if allowed, ok := imageModelSizes[r.Model]; ok {
		if r.Size == "" {
			return nil
		}
		for _, s := range allowed {
			if s == r.Size {
				return nil
			}
		}
		return fmt.Errorf("size must be one of %s for %s", strings.Join(allowed, ", "), r.Model)
	}
	if r.Size == "" || r.Size == "auto" {
		return nil
	}
	if !sizePattern.MatchString(r.Size) {
		return fmt.Errorf("invalid size %q: expected <width>x<height>", r.Size)
	}
	return nil
}

// ApplyModelDefaults fills the model-specific defaults of the parsed
// request: the quality defaults (dall-e-3=standard, gpt-image-1=auto) so
// the price ratio of a defaulted request is explicit (P3.1 validation
// rules). Call it after the model is resolved (including the distributor
// "dall-e" fallback).
func (r *ImageRequest) ApplyModelDefaults() {
	switch r.Model {
	case "dall-e-3":
		if r.Quality == "" {
			r.Quality = "standard"
		}
	case "gpt-image-1":
		if r.Quality == "" {
			r.Quality = "auto"
		}
	}
}

// ParseImageRequest parses a JSON image generation body. Unknown fields are
// preserved in Extra for upstream passthrough. A missing model is not an
// error here: the generations endpoint applies the distributor default
// (P3.1 "dall-e" fallback); edits extracts the model from the multipart
// form and enforces it itself.
func ParseImageRequest(body []byte) (*ImageRequest, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	req := &ImageRequest{Extra: map[string]json.RawMessage{}}
	for k, v := range raw {
		switch k {
		case "model":
			if err := json.Unmarshal(v, &req.Model); err != nil {
				return nil, fmt.Errorf("invalid %s: %w", k, err)
			}
		case "prompt":
			if err := json.Unmarshal(v, &req.Prompt); err != nil {
				return nil, fmt.Errorf("invalid %s: %w", k, err)
			}
		case "n":
			if err := json.Unmarshal(v, &req.N); err != nil {
				return nil, fmt.Errorf("invalid n: %w", err)
			}
		case "size":
			_ = json.Unmarshal(v, &req.Size)
		case "quality":
			_ = json.Unmarshal(v, &req.Quality)
		case "style":
			_ = json.Unmarshal(v, &req.Style)
		case "response_format":
			_ = json.Unmarshal(v, &req.ResponseFormat)
		case "stream":
			_ = json.Unmarshal(v, &req.Stream)
		case "watermark":
			_ = json.Unmarshal(v, &req.Watermark)
		case "user":
			_ = json.Unmarshal(v, &req.User)
		default:
			req.Extra[k] = v
		}
	}
	if req.Prompt == "" {
		return nil, errors.New("the \"prompt\" field is required")
	}
	if err := req.ValidateN(); err != nil {
		return nil, err
	}
	if err := req.ValidateSize(); err != nil {
		return nil, err
	}
	return req, nil
}

// MarshalBody serializes the request back to JSON, flattening Extra fields.
func (r *ImageRequest) MarshalBody() ([]byte, error) {
	base, err := json.Marshal(struct {
		Model          string `json:"model"`
		Prompt         string `json:"prompt"`
		N              *uint  `json:"n,omitempty"`
		Size           string `json:"size,omitempty"`
		Quality        string `json:"quality,omitempty"`
		Style          string `json:"style,omitempty"`
		ResponseFormat string `json:"response_format,omitempty"`
		Stream         *bool  `json:"stream,omitempty"`
		Watermark      *bool  `json:"watermark,omitempty"`
		User           string `json:"user,omitempty"`
	}{r.Model, r.Prompt, r.N, r.Size, r.Quality, r.Style, r.ResponseFormat, r.Stream, r.Watermark, r.User})
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

// ImageResponseData is one generated image.
type ImageResponseData struct {
	URL           string `json:"url,omitempty"`
	B64JSON       string `json:"b64_json,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

// ImageUsage is the gpt-image-1 style usage block (note: input_tokens /
// output_tokens names, normalized to Prompt/Completion by the image relay
// only — the overwrite semantics must not leak into chat/embedding paths).
type ImageUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
	TotalTokens  int64 `json:"total_tokens"`
}

// ImageResponse is an image API response.
type ImageResponse struct {
	Created int64               `json:"created"`
	Data    []ImageResponseData `json:"data"`
	Usage   *ImageUsage         `json:"usage,omitempty"`
}

// Count returns the number of images actually delivered: it is the larger
// of the url-bearing and the b64_json-bearing entry counts. A standard
// response uses one response_format so this equals len(Data); an upstream
// that splits one image into a url entry and a b64_json entry bills once,
// and payload-less entries bill nothing. A zero count leaves the requested
// quantity in place at settlement (the [1, MaxImageN] guard).
func (r *ImageResponse) Count() int {
	if r == nil {
		return 0
	}
	urls, b64s := 0, 0
	for _, img := range r.Data {
		if img.URL != "" {
			urls++
		}
		if img.B64JSON != "" {
			b64s++
		}
	}
	return max(urls, b64s)
}

// HasPayload reports whether one data entry carries an image.
func (d ImageResponseData) HasPayload() bool {
	return d.URL != "" || d.B64JSON != ""
}

// ParseImageResponse parses an upstream image response (best effort: the
// body is relayed verbatim to the client; this is only for billing).
func ParseImageResponse(body []byte) (*ImageResponse, error) {
	var resp ImageResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ImageStreamEvent is one parsed image SSE event.
type ImageStreamEvent struct {
	Type         string // image_generation.partial_image | image_generation.completed | error
	OutputIndex  int
	B64JSON      string
	Usage        *ImageUsage
	ErrorMessage string
}

// ParseImageStreamEvent parses one image stream event payload.
func ParseImageStreamEvent(eventName, payload []byte) (*ImageStreamEvent, error) {
	var ev struct {
		Type        string      `json:"type"`
		OutputIndex int         `json:"output_index"`
		B64JSON     string      `json:"b64_json"`
		Usage       *ImageUsage `json:"usage"`
		Error       *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(payload, &ev); err != nil {
		return nil, err
	}
	out := &ImageStreamEvent{
		Type:        ev.Type,
		OutputIndex: ev.OutputIndex,
		B64JSON:     ev.B64JSON,
		Usage:       ev.Usage,
	}
	if out.Type == "" {
		out.Type = string(eventName)
	}
	if ev.Error != nil {
		out.Type = "error"
		out.ErrorMessage = ev.Error.Message
	}
	return out, nil
}

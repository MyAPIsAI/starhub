package dto

import (
	"bytes"
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

// ImageEntryRaw carries the raw (still JSON-encoded, quotes included)
// url and b64_json values of one data entry.
//
// The slices alias the caller's response body:
// they stay valid for the duration of the WalkImageResponse call (and
// the emit callback inside it) and must not be retained (P7.1: image
// bytes are counted and forwarded, never duplicated).
type ImageEntryRaw struct {
	URL json.RawMessage `json:"url"`
	B64 json.RawMessage `json:"b64_json"`
}

// rawStringClass classifies a raw JSON value the way a string-field
// unmarshal would: absent and null are "empty", "" is empty, any other
// string is non-empty, and a non-string value is a type error (the
// legacy whole-response parse failed on those).
func rawStringClass(raw json.RawMessage) (nonEmpty, typeOK bool) {
	t := bytes.TrimSpace(raw)
	if len(t) == 0 || string(t) == "null" {
		return false, true
	}
	if len(t) >= 2 && t[0] == '"' && t[len(t)-1] == '"' {
		return len(t) > 2, true
	}
	return false, false
}

// RawStringLiteral returns the raw value as a JSON string literal:
// absent / null / empty fall back to "" (matching what the legacy
// re-marshal emitted for a missing payload), anything else passes
// through verbatim with its quotes.
func RawStringLiteral(raw json.RawMessage) []byte {
	t := bytes.TrimSpace(raw)
	if len(t) == 0 || string(t) == "null" {
		return []byte(`""`)
	}
	return t
}

// WalkImageResponse streams the data entries of an upstream image
// response without materializing the payload strings. Counting
// semantics mirror ImageResponse.Count exactly: an entry counts once
// per non-empty url / b64_json value and the response count is
// max(url-entries, b64-entries); payload-less entries bill nothing.
//
// The body is the only payload-scale buffer alive during the walk;
// entry url/b64 values are subslices, so emit receives them without copying.
//
// Accepted divergence from the legacy whole-struct parse: entry members
// other than url/b64_json (e.g. a malformed revised_prompt) are not
// type-checked; such a response settles on the delivered payload count
// instead of 0 — the revenue-safe direction.
func WalkImageResponse(body []byte, emit func(entry ImageEntryRaw) error) (urls, b64s int, usage *ImageUsage, err error) {
	var head struct {
		Created int64         `json:"created"`
		Data    imageJSONView `json:"data"`
		Usage   *ImageUsage   `json:"usage"`
	}
	if err := json.Unmarshal(body, &head); err != nil {
		return 0, 0, nil, err
	}
	err = walkImageEntries(head.Data, func(entry []byte) error {
		url, b64, err := extractImageEntryFields(entry)
		if err != nil {
			return err
		}
		urlNonEmpty, ok := rawStringClass(url)
		if !ok {
			return errors.New("\"url\" is not a string")
		}
		b64NonEmpty, ok := rawStringClass(b64)
		if !ok {
			return errors.New("\"b64_json\" is not a string")
		}
		if urlNonEmpty {
			urls++
		}
		if b64NonEmpty {
			b64s++
		}
		if emit != nil {
			return emit(ImageEntryRaw{URL: url, B64: b64})
		}
		return nil
	})
	if err != nil {
		return 0, 0, nil, err
	}
	return urls, b64s, head.Usage, nil
}

// imageJSONView borrows a validated token from json.Unmarshal's input.
// It is private to the synchronous response walk: unlike RawMessage it
// must never escape the input body's lifetime or be used with a decoder
// that reuses its input buffer.
type imageJSONView []byte

func (v *imageJSONView) UnmarshalJSON(data []byte) error {
	*v = data
	return nil
}

// walkImageEntries iterates the entries of the data array. The array is
// a validated JSON token (captured by the top-level Unmarshal), so the
// navigation below is structural only. "data": null means no entries
// (legacy parity: a null array unmarshals to an empty slice).
func walkImageEntries(data []byte, onEntry func(entry []byte) error) error {
	i := skipJSONWhitespace(data, 0)
	if i >= len(data) || data[i] == 'n' { // missing or null
		return nil
	}
	if data[i] != '[' {
		return errors.New("\"data\" is not an array")
	}
	i = skipJSONWhitespace(data, i+1)
	if i < len(data) && data[i] == ']' {
		return nil
	}
	for {
		if i >= len(data) || data[i] != '{' {
			return errors.New("image data entry is not an object")
		}
		end, err := scanJSONValueEnd(data, i)
		if err != nil {
			return err
		}
		if err := onEntry(data[i:end]); err != nil {
			return err
		}
		i = skipJSONWhitespace(data, end)
		if i < len(data) && data[i] == ',' {
			i = skipJSONWhitespace(data, i+1)
			continue
		}
		if i < len(data) && data[i] == ']' {
			return nil
		}
		return errors.New("malformed image data array")
	}
}

// extractImageEntryFields returns the raw value spans of the url and
// b64_json members of one entry object. Spans alias entry; a duplicate
// key keeps the last occurrence (json.Unmarshal semantics).
func extractImageEntryFields(entry []byte) (url, b64 json.RawMessage, err error) {
	i := skipJSONWhitespace(entry, 1) // entry[0] == '{'
	if i < len(entry) && entry[i] == '}' {
		return nil, nil, nil
	}
	for {
		if i >= len(entry) || entry[i] != '"' {
			return nil, nil, errors.New("malformed image data entry")
		}
		keyEnd := scanJSONStringEnd(entry, i)
		key := string(entry[i+1 : keyEnd-1])
		i = skipJSONWhitespace(entry, keyEnd)
		if i >= len(entry) || entry[i] != ':' {
			return nil, nil, errors.New("malformed image data entry")
		}
		valStart := skipJSONWhitespace(entry, i+1)
		valEnd, err := scanJSONValueEnd(entry, valStart)
		if err != nil {
			return nil, nil, err
		}
		switch key {
		case "url":
			url = entry[valStart:valEnd]
		case "b64_json":
			b64 = entry[valStart:valEnd]
		}
		i = skipJSONWhitespace(entry, valEnd)
		if i < len(entry) && entry[i] == ',' {
			i = skipJSONWhitespace(entry, i+1)
			continue
		}
		if i < len(entry) && entry[i] == '}' {
			return url, b64, nil
		}
		return nil, nil, errors.New("malformed image data entry")
	}
}

// skipJSONWhitespace advances past JSON whitespace.
func skipJSONWhitespace(b []byte, i int) int {
	for i < len(b) {
		switch b[i] {
		case ' ', '\t', '\r', '\n':
			i++
		default:
			return i
		}
	}
	return i
}

// scanJSONValueEnd returns the index just past the JSON value starting
// at i (strings honor escapes, containers nest).
func scanJSONValueEnd(b []byte, i int) (int, error) {
	if i >= len(b) {
		return 0, errors.New("unexpected end of JSON value")
	}
	switch b[i] {
	case '"':
		return scanJSONStringEnd(b, i), nil
	case '{', '[':
		depth := 0
		j := i
		for j < len(b) {
			switch b[j] {
			case '"':
				j = scanJSONStringEnd(b, j)
				continue
			case '{', '[':
				depth++
			case '}', ']':
				depth--
				if depth == 0 {
					return j + 1, nil
				}
			}
			j++
		}
		return 0, errors.New("unterminated JSON container")
	}
	// number / true / false / null
	j := i
	for j < len(b) {
		switch b[j] {
		case ',', '}', ']', ' ', '\t', '\r', '\n':
			return j, nil
		}
		j++
	}
	return j, nil
}

// scanJSONStringEnd returns the index just past the string starting at
// b[i] (b[i] must be the opening quote).
func scanJSONStringEnd(b []byte, i int) int {
	j := i + 1
	for j < len(b) {
		switch b[j] {
		case '\\':
			j += 2
		case '"':
			return j + 1
		default:
			j++
		}
	}
	return j
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

// ImageStreamEventMeta is the billing-relevant subset of an image stream
// event. The megabyte-scale b64_json payload is deliberately absent:
// meta parsing scans it but never materializes it (P7.1 memory
// discipline — the forwarding path already holds the payload bytes).
type ImageStreamEventMeta struct {
	Type         string // image_generation.partial_image | image_generation.completed | error
	OutputIndex  int
	Usage        *ImageUsage
	ErrorMessage string
}

// ParseImageStreamEventMeta parses the billing fields of one image
// stream event. Semantics match ParseImageStreamEvent for every field
// except the omitted B64JSON.
func ParseImageStreamEventMeta(eventName, payload []byte) (*ImageStreamEventMeta, error) {
	var ev struct {
		Type        string      `json:"type"`
		OutputIndex int         `json:"output_index"`
		Usage       *ImageUsage `json:"usage"`
		Error       *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(payload, &ev); err != nil {
		return nil, err
	}
	out := &ImageStreamEventMeta{
		Type:        ev.Type,
		OutputIndex: ev.OutputIndex,
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

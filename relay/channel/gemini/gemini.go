// Package gemini implements the Google Gemini channel adaptor: the P3.1
// representative of a "different path style" provider (per-model
// /v1beta/models/{model}:generateContent endpoints and the
// x-goog-api-key header).
package gemini

import (
	"io"
	"net/http"
	"strings"

	"starhub/relay/channel"
	"starhub/relay/dto"
)

const (
	defaultBaseURL   = "https://generativelanguage.googleapis.com"
	maxResponseBytes = 32 << 20
)

type Adaptor struct{}

// New returns the stateless Gemini adaptor.
func New() *Adaptor { return &Adaptor{} }

func (Adaptor) Name() string { return "gemini" }

func (Adaptor) NativeInboundFormat() dto.InboundFormat { return dto.InboundGemini }

func (Adaptor) ChatURL(baseURL, _, model string, stream bool) string {
	method := "generateContent"
	if stream {
		method = "streamGenerateContent?alt=sse"
	}
	return strings.TrimSuffix(trimBase(baseURL), "/v1beta") + "/v1beta/models/" + model + ":" + method
}

func (Adaptor) ModeURL(_ string, _ channel.RelayMode) (string, error) {
	return "", channel.ErrUnsupportedMode
}

func (Adaptor) RealtimeURL(_, _ string) (string, error) {
	return "", channel.ErrUnsupportedMode
}

func (Adaptor) PrepareRequest(req *http.Request, apiKey string, _ []byte) error {
	if apiKey != "" {
		req.Header.Set("x-goog-api-key", apiKey)
	}
	return nil
}

// PreparePassthroughBody retains native content without chat-only options.
func (Adaptor) PreparePassthroughBody(body []byte, _ bool) ([]byte, error) {
	return body, nil
}

func (Adaptor) ConvertChatRequest(ir *dto.ChatRequest) ([]byte, error) {
	return dto.BuildGeminiChatRequest(ir)
}

// HandlePassthroughResponse retains the native JSON envelope.
func (Adaptor) HandlePassthroughResponse(w http.ResponseWriter, resp *http.Response, _ bool) (*dto.Usage, string, error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, "", err
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
	ir, err := dto.ParseGeminiChatResponse(body)
	if err != nil {
		return nil, "", nil
	}
	return ir.Usage, ir.Text, nil
}

// ParseChatResponse reads a non-stream Gemini response into IR.
func (Adaptor) ParseChatResponse(resp *http.Response) (*dto.ChatResponse, error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, err
	}
	return dto.ParseGeminiChatResponse(body)
}

// StreamChatEvents wraps a streamGenerateContent?alt=sse response into IR
// events.
func (Adaptor) StreamChatEvents(resp *http.Response) (channel.ChatStreamReader, error) {
	return &streamReader{reader: channel.NewSSEReader(resp.Body), closer: resp.Body}, nil
}

type streamReader struct {
	reader *channel.SSEReader
	closer io.Closer
}

func (s *streamReader) Next() (*dto.ChatStreamEvent, error) {
	for {
		line, err := s.reader.Next()
		if err != nil {
			return nil, err
		}
		if line.Data == "" {
			continue
		}
		ev, perr := dto.ParseGeminiStreamPayload([]byte(line.Data))
		if perr != nil {
			continue
		}
		return ev, nil
	}
}

func (s *streamReader) Close() error {
	if s.closer != nil {
		return s.closer.Close()
	}
	return nil
}

func trimBase(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = defaultBaseURL
	}
	return base
}

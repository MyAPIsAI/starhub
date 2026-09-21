// Package claude implements the Anthropic Claude channel adaptor: the
// P3.1 representative of a "different auth header" provider (x-api-key +
// anthropic-version instead of a bearer token).
package claude

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"starhub/relay/channel"
	"starhub/relay/dto"
)

const (
	defaultBaseURL   = "https://api.anthropic.com"
	anthropicVersion = "2023-06-01"
	maxResponseBytes = 32 << 20
)

type Adaptor struct{}

// New returns the stateless Claude adaptor.
func New() *Adaptor { return &Adaptor{} }

func (Adaptor) Name() string                           { return "claude" }
func (Adaptor) NativeInboundFormat() dto.InboundFormat { return dto.InboundClaude }

func (Adaptor) ChatURL(baseURL, _, _ string, _ bool) string {
	return trimBase(baseURL) + "/v1/messages"
}

func (Adaptor) ModeURL(_ string, _ channel.RelayMode) (string, error) {
	return "", channel.ErrUnsupportedMode
}

func (Adaptor) RealtimeURL(_, _ string) (string, error) {
	return "", channel.ErrUnsupportedMode
}

func (Adaptor) PrepareRequest(req *http.Request, apiKey string, _ []byte) error {
	if apiKey != "" {
		req.Header.Set("x-api-key", apiKey)
	}
	req.Header.Set("anthropic-version", anthropicVersion)
	return nil
}

// PreparePassthroughBody forwards a Claude inbound body unchanged (Claude
// streams always carry usage in message_start / message_delta).
func (Adaptor) PreparePassthroughBody(body []byte, _ bool) ([]byte, error) {
	return body, nil
}

func (Adaptor) ConvertChatRequest(ir *dto.ChatRequest) ([]byte, error) {
	return dto.BuildClaudeChatRequest(ir)
}

// HandlePassthroughResponse relays a native Claude response to the client
// while extracting usage for settlement.
func (Adaptor) HandlePassthroughResponse(w http.ResponseWriter, resp *http.Response, stream bool) (*dto.Usage, string, error) {
	if !stream {
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
		if err != nil {
			return nil, "", err
		}
		writeJSONHead(w, resp)
		_, _ = w.Write(body)
		ir, err := dto.ParseClaudeChatResponse(body)
		if err != nil {
			return nil, "", nil
		}
		return ir.Usage, ir.Text, nil
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	flusher, canFlush := w.(http.Flusher)

	reader := channel.NewSSEReader(resp.Body)
	var usage *dto.Usage
	var inputTokens int64
	var content strings.Builder
	for {
		line, err := reader.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return usage, content.String(), err
		}
		if line.Event != "" {
			fmt.Fprintf(w, "event: %s\n", line.Event)
		}
		if line.Data != "" {
			fmt.Fprintf(w, "data: %s\n", line.Data)
		}
		fmt.Fprint(w, "\n")
		if canFlush {
			flusher.Flush()
		}
		if line.Data == "" {
			continue
		}
		ev, perr := dto.ParseClaudeStreamEvent([]byte(line.Data))
		if perr != nil {
			continue
		}
		switch ev.Type {
		case "message_start":
			if ev.Usage != nil {
				inputTokens = ev.Usage.PromptTokens
			}
		case "content_block_delta":
			content.WriteString(ev.TextDelta)
		case "message_delta", "message_stop":
			if ev.Usage != nil {
				// message_delta usage reports the output side only: merge
				// with the message_start input count instead of dropping it.
				if usage == nil {
					usage = ev.Usage
				} else {
					if ev.Usage.PromptTokens > 0 {
						usage.PromptTokens = ev.Usage.PromptTokens
					}
					if ev.Usage.CompletionTokens > 0 {
						usage.CompletionTokens = ev.Usage.CompletionTokens
					}
				}
			}
		}
	}
	if usage == nil && inputTokens > 0 {
		usage = &dto.Usage{PromptTokens: inputTokens}
	} else if usage != nil && usage.PromptTokens == 0 {
		usage.PromptTokens = inputTokens
	}
	return usage, content.String(), nil
}

// ParseChatResponse reads a non-stream Claude response into IR.
func (Adaptor) ParseChatResponse(resp *http.Response) (*dto.ChatResponse, error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, err
	}
	return dto.ParseClaudeChatResponse(body)
}

// StreamChatEvents wraps a Claude SSE stream into IR events.
func (Adaptor) StreamChatEvents(resp *http.Response) (channel.ChatStreamReader, error) {
	return &streamReader{reader: channel.NewSSEReader(resp.Body), closer: resp.Body}, nil
}

type streamReader struct {
	reader *channel.SSEReader
	closer io.Closer
	// pendingInput holds the message_start usage until the final
	// message_delta completes the report.
	pendingInput *dto.Usage
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
		ev, perr := dto.ParseClaudeStreamEvent([]byte(line.Data))
		if perr != nil {
			continue
		}
		switch ev.Type {
		case "message_start":
			// The input usage arrives here; hold it until the final
			// message_delta completes the report.
			s.pendingInput = ev.Usage
			continue
		case "content_block_delta", "message_delta", "message_stop":
		default:
			continue // ping / content_block_start / content_block_stop
		}
		out := &dto.ChatStreamEvent{
			TextDelta:    ev.TextDelta,
			FinishReason: openAIFinishReason(ev.StopReason),
		}
		if ev.Usage != nil {
			if s.pendingInput != nil && ev.Usage.PromptTokens == 0 {
				ev.Usage.PromptTokens = s.pendingInput.PromptTokens
			}
			out.Usage = ev.Usage
			s.pendingInput = nil
		}
		return out, nil
	}
}

func (s *streamReader) Close() error {
	if s.closer != nil {
		return s.closer.Close()
	}
	return nil
}

func openAIFinishReason(claudeReason string) string {
	switch claudeReason {
	case "end_turn", "stop_sequence":
		return "stop"
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	}
	return claudeReason
}

func writeJSONHead(w http.ResponseWriter, resp *http.Response) {
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
}

func trimBase(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = defaultBaseURL
	}
	return base
}

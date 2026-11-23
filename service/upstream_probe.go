package service

import (
	"encoding/json"
	"io"
	"net/http"

	"starhub/internal/upstream"
	"starhub/model"
	"starhub/relay/channel"
	"starhub/relay/dto"
)

func protectionProbePath(s upstream.State) string {
	switch s.Capability {
	case "chat", "claude":
		return "/v1/chat/completions"
	case "gemini":
		return "/v1beta/models/" + s.Model + ":generateContent"
	case "responses":
		return "/v1/responses"
	case "realtime":
		return "/v1/realtime"
	default:
		return ""
	}
}

func protectionProbeRequest(adaptor channel.ChatAdaptor, ch *model.Channel, s upstream.State, key string) ([]byte, string, error) {
	if s.Capability == "responses" {
		url, err := adaptor.ModeURL(ch.BaseURL, channel.RelayModeResponses)
		if err != nil {
			return nil, "", err
		}
		body, err := json.Marshal(map[string]any{"model": s.Model, "input": "Reply OK.", "max_output_tokens": 16, "stream": s.Stream, "store": false, "background": false})
		return body, url, err
	}
	body, err := adaptor.ConvertChatRequest(&dto.ChatRequest{Model: s.Model, Messages: []dto.ChatMessage{{Role: "user", Text: "Reply OK."}}, MaxTokens: 16, Stream: s.Stream})
	return body, adaptor.ChatURL(ch.BaseURL, key, s.Model, s.Stream), err
}

func validProbeResponse(body []byte) bool {
	var result struct {
		ID     string            `json:"id"`
		Object string            `json:"object"`
		Status string            `json:"status"`
		Output []json.RawMessage `json:"output"`
	}
	if json.Unmarshal(body, &result) != nil || result.ID == "" || result.Object != "response" || (result.Status != "completed" && result.Status != "incomplete") {
		return false
	}
	usage, err := dto.ParseResponsesUsage(body)
	return err == nil && usage != nil && (len(result.Output) > 0 || usage.TotalTokens() > 0)
}

func protectionProbeComplete(adaptor channel.ChatAdaptor, s upstream.State, resp *http.Response) bool {
	// Bound probe memory even when an upstream ignores the output token limit.
	limit := &io.LimitedReader{R: resp.Body, N: 4 << 20}
	resp.Body = struct {
		io.Reader
		io.Closer
	}{limit, resp.Body}
	if s.Capability == "responses" {
		if !s.Stream {
			body, err := io.ReadAll(resp.Body)
			return err == nil && limit.N > 0 && validProbeResponse(body)
		}
		reader := channel.NewSSEReader(resp.Body)
		for {
			frame, err := reader.Next()
			if err != nil || limit.N <= 0 {
				return false
			}
			var event struct {
				Type     string          `json:"type"`
				Response json.RawMessage `json:"response"`
			}
			if json.Unmarshal([]byte(frame.Data), &event) != nil {
				return false
			}
			switch event.Type {
			case "response.completed", "response.incomplete":
				return validProbeResponse(event.Response)
			case "error", "response.failed", "response.cancelled":
				return false
			}
		}
	}
	if s.Stream {
		reader, err := adaptor.StreamChatEvents(resp)
		if err != nil {
			return false
		}
		defer reader.Close()
		finished, content := false, false
		for {
			event, err := reader.Next()
			if err == io.EOF {
				return limit.N > 0 && finished && content
			}
			if err != nil || limit.N <= 0 {
				return false
			}
			if event != nil {
				finished = finished || event.FinishReason != ""
				content = content || event.TextDelta != "" || event.Usage != nil && event.Usage.TotalTokens() > 0
			}
		}
	}
	parsed, err := adaptor.ParseChatResponse(resp)
	return err == nil && limit.N > 0 && parsed != nil && parsed.FinishReason != "" && (parsed.Text != "" || len(parsed.ToolCalls) > 0 || parsed.Usage != nil && parsed.Usage.TotalTokens() > 0)
}

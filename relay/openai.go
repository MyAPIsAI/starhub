package relay

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

const (
	defaultOpenAIBaseURL = "https://api.openai.com"
	maxResponseBytes     = 32 << 20
)

type openaiAdaptor struct{}

func (openaiAdaptor) Name() string {
	return "openai"
}

func (openaiAdaptor) ConvertRequestBody(body []byte, stream bool) ([]byte, error) {
	if !stream {
		return body, nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, err
	}
	if _, ok := m["stream_options"]; !ok {
		m["stream_options"] = json.RawMessage(`{"include_usage":true}`)
	}
	return json.Marshal(m)
}

func (openaiAdaptor) RequestURL(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = defaultOpenAIBaseURL
	}
	return base + "/v1/chat/completions"
}

func (openaiAdaptor) SetupRequestHeader(req *http.Request, channelKey string) {
	req.Header.Set("Content-Type", "application/json")
	if channelKey != "" {
		req.Header.Set("Authorization", "Bearer "+channelKey)
	}
}

func (openaiAdaptor) HandleResponse(w http.ResponseWriter, resp *http.Response, stream bool) (*Usage, string, error) {
	if !stream {
		return handleNonStreamResponse(w, resp)
	}
	return handleStreamResponse(w, resp)
}

func handleNonStreamResponse(w http.ResponseWriter, resp *http.Response) (*Usage, string, error) {
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
	var parsed struct {
		Usage   *Usage `json:"usage"`
		Choices []struct {
			Message *struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, "", nil
	}
	var content string
	if len(parsed.Choices) > 0 && parsed.Choices[0].Message != nil {
		content = contentText(parsed.Choices[0].Message.Content)
	}
	return parsed.Usage, content, nil
}

func handleStreamResponse(w http.ResponseWriter, resp *http.Response) (*Usage, string, error) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	flusher, canFlush := w.(http.Flusher)

	reader := bufio.NewReaderSize(resp.Body, 64*1024)
	var usage *Usage
	var content strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			if _, werr := w.Write([]byte(line)); werr != nil {
				return usage, content.String(), werr
			}
			if canFlush {
				flusher.Flush()
			}
			observeStreamLine(line, &usage, &content)
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return usage, content.String(), err
		}
	}
	return usage, content.String(), nil
}

func observeStreamLine(line string, usage **Usage, content *strings.Builder) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "data:") {
		return
	}
	payload := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
	if payload == "" || payload == "[DONE]" {
		return
	}
	var chunk struct {
		Usage   *Usage `json:"usage"`
		Choices []struct {
			Delta *struct {
				Content json.RawMessage `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		return
	}
	if chunk.Usage != nil && (chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0) {
		*usage = chunk.Usage
	}
	if len(chunk.Choices) > 0 && chunk.Choices[0].Delta != nil {
		if text := contentText(chunk.Choices[0].Delta.Content); text != "" {
			content.WriteString(text)
		}
	}
}

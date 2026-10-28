// Package openai implements the OpenAI-compatible channel adaptor: the
// native protocol of the gateway (chat passthrough) plus every passthrough
// relay mode (embeddings, rerank, audio, images) and the Sora-style video
// task platform.
package openai

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"starhub/relay/channel"
	"starhub/relay/dto"
	"starhub/setting"
)

const (
	defaultBaseURL   = "https://api.openai.com"
	maxResponseBytes = 32 << 20
)

type Adaptor struct{}

// New returns the stateless OpenAI adaptor.
func New() *Adaptor { return &Adaptor{} }

func (Adaptor) Name() string                           { return "openai" }
func (Adaptor) NativeInboundFormat() dto.InboundFormat { return dto.InboundOpenAI }

func (Adaptor) ChatURL(baseURL, _, _ string, _ bool) string {
	return trimBase(baseURL) + "/v1/chat/completions"
}

func (Adaptor) ModeURL(baseURL string, mode channel.RelayMode) (string, error) {
	base := trimBase(baseURL)
	switch mode {
	case channel.RelayModeResponses:
		return strings.TrimSuffix(base, "/v1") + "/v1/responses", nil
	case channel.RelayModeEmbeddings:
		return base + "/v1/embeddings", nil
	case channel.RelayModeRerank:
		return base + "/v1/rerank", nil
	case channel.RelayModeTTS:
		return base + "/v1/audio/speech", nil
	case channel.RelayModeSTT:
		return base + "/v1/audio/transcriptions", nil
	case channel.RelayModeSTTTranslation:
		return base + "/v1/audio/translations", nil
	case channel.RelayModeImageGeneration:
		return base + "/v1/images/generations", nil
	case channel.RelayModeImageEdit:
		return base + "/v1/images/edits", nil
	default:
		return "", channel.ErrUnsupportedMode
	}
}

func (Adaptor) RealtimeURL(baseURL, model string) (string, error) {
	return trimBase(baseURL) + "/v1/realtime?model=" + model, nil
}

func (Adaptor) PrepareRequest(req *http.Request, apiKey string, _ []byte) error {
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	return nil
}

// PreparePassthroughBody injects stream_options.include_usage so a streamed
// chat response always carries a usage report for settlement.
func (Adaptor) PreparePassthroughBody(body []byte, stream bool) ([]byte, error) {
	if !stream {
		return body, nil
	}
	return injectStreamOptions(body)
}

func (Adaptor) ConvertChatRequest(ir *dto.ChatRequest) ([]byte, error) {
	body, err := dto.BuildOpenAIChatRequest(ir)
	if err != nil {
		return nil, err
	}
	if ir.Stream {
		// Converted streams deserve the same usage guarantee as
		// passthrough ones.
		return injectStreamOptions(body)
	}
	return body, nil
}

// injectStreamOptions adds stream_options.include_usage to a JSON body.
func injectStreamOptions(body []byte) ([]byte, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, err
	}
	if _, ok := m["stream_options"]; !ok {
		m["stream_options"] = json.RawMessage(`{"include_usage":true}`)
	}
	return json.Marshal(m)
}

// HandlePassthroughResponse copies a native OpenAI response to the client
// while observing usage / content for settlement.
func (Adaptor) HandlePassthroughResponse(w http.ResponseWriter, resp *http.Response, stream bool) (*dto.Usage, string, error) {
	if !stream {
		return handleNonStream(w, resp)
	}
	return handleStream(w, resp)
}

func handleNonStream(w http.ResponseWriter, resp *http.Response) (*dto.Usage, string, error) {
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
	ir, err := dto.ParseOpenAIChatResponse(body)
	if err != nil {
		return nil, "", nil // unreadable body: relayed verbatim, billed by estimate
	}
	return ir.Usage, ir.Text, nil
}

func handleStream(w http.ResponseWriter, resp *http.Response) (*dto.Usage, string, error) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	flusher, canFlush := w.(http.Flusher)

	reader := channel.NewSSEReader(resp.Body)
	var usage *dto.Usage
	var content strings.Builder
	for {
		line, err := reader.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return usage, content.String(), err
		}
		payload := line.Data
		if payload == "" || payload == "[DONE]" {
			continue
		}
		textDelta, chunkUsage, _, perr := dto.ParseOpenAIStreamPayload([]byte(payload))
		if perr == nil {
			if chunkUsage != nil && chunkUsage.TotalTokens() > 0 {
				usage = chunkUsage
			}
			content.WriteString(textDelta)
		}
		// Re-emit the frame byte-faithfully (one data: line per event).
		if _, werr := fmt.Fprintf(w, "data: %s\n\n", payload); werr != nil {
			return usage, content.String(), werr
		}
		if canFlush {
			flusher.Flush()
		}
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	fmt.Fprint(w, "data: [DONE]\n\n")
	if canFlush {
		flusher.Flush()
	}
	return usage, content.String(), nil
}

// ParseChatResponse reads a non-stream OpenAI response into IR.
func (Adaptor) ParseChatResponse(resp *http.Response) (*dto.ChatResponse, error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, err
	}
	return dto.ParseOpenAIChatResponse(body)
}

// StreamChatEvents wraps an OpenAI SSE stream into IR events.
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
		payload := line.Data
		if payload == "" || payload == "[DONE]" {
			continue
		}
		textDelta, usage, finish, perr := dto.ParseOpenAIStreamPayload([]byte(payload))
		if perr != nil {
			continue // tolerate keepalives and non-JSON frames
		}
		return &dto.ChatStreamEvent{
			TextDelta:    textDelta,
			FinishReason: finish,
			Usage:        usage,
		}, nil
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

// ---------------------------------------------------------------------------
// Sora-style video task platform (P3.3 reference implementation)
// ---------------------------------------------------------------------------

// VideoTaskAdaptor submits/polls OpenAI Sora-style video generation tasks
// (POST /v1/videos, GET /v1/videos/{id}). The billing model is USD per
// second x n videos.
type VideoTaskAdaptor struct{}

func (VideoTaskAdaptor) Name() string     { return "openai-video" }
func (VideoTaskAdaptor) Platform() string { return "sora" }

func (VideoTaskAdaptor) ValidateRequestAndSetAction(body []byte, req *dto.TaskRequest) error {
	parsed, err := dto.ParseVideoTaskRequest(body)
	if err != nil {
		return err
	}
	*req = *parsed
	return nil
}

func (VideoTaskAdaptor) SubmitURL(baseURL string) string {
	return trimBase(baseURL) + "/v1/videos"
}

func (VideoTaskAdaptor) FetchURL(baseURL, taskID string) string {
	return trimBase(baseURL) + "/v1/videos/" + taskID
}

func (VideoTaskAdaptor) PrepareRequest(req *http.Request, apiKey string, _ []byte) error {
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	return nil
}

func (VideoTaskAdaptor) ConvertSubmitRequest(req *dto.TaskRequest) ([]byte, error) {
	return json.Marshal(map[string]any{
		"model":   req.Model,
		"prompt":  req.Prompt,
		"seconds": req.Seconds,
		"size":    req.Size,
		"n":       req.N,
	})
}

func (VideoTaskAdaptor) ParseSubmitResponse(statusCode int, body []byte) (string, error) {
	if statusCode >= http.StatusBadRequest {
		if env := dto.ParseProviderErrorBody(body); env != nil {
			return "", fmt.Errorf("upstream %d: %s", statusCode, env.Message)
		}
		return "", fmt.Errorf("upstream returned %d", statusCode)
	}
	return dto.ParseVideoTaskSubmitResponse(body)
}

func (VideoTaskAdaptor) ParseTaskResult(statusCode int, body []byte) (*dto.TaskResult, error) {
	if statusCode >= http.StatusBadRequest {
		if env := dto.ParseProviderErrorBody(body); env != nil {
			return nil, fmt.Errorf("upstream %d: %s", statusCode, env.Message)
		}
		return nil, fmt.Errorf("upstream returned %d", statusCode)
	}
	return dto.ParseVideoTaskResult(body)
}

// soraPricePerSecond is the default USD-per-second price used when the
// operator has not configured model_price for a video model.
const soraPricePerSecond = 0.1

func (VideoTaskAdaptor) EstimateBilling(modelName, group string, req *dto.TaskRequest) channel.TaskPricing {
	price, set := setting.ModelPriceValue(modelName)
	expression, _ := setting.ModelPriceExpr(modelName)
	return (VideoTaskAdaptor{}).EstimateBillingWithOptions(modelName, group, req, channel.TaskPricingOptions{ModelPriceUSD: price, ModelPriceSet: set, Expression: expression})
}

func (VideoTaskAdaptor) EstimateBillingWithOptions(modelName, group string, req *dto.TaskRequest, options channel.TaskPricingOptions) channel.TaskPricing {
	// Expression path: when model_price_expr is configured, fill ExprVars
	// with the request parameters. Pre-charge assumes task_status=1 (full
	// success price); the settle step updates it with the terminal status.
	if options.Expression != "" {
		promptEst := dto.EstimateTokens(req.Prompt) + 100
		return channel.TaskPricing{
			ExprVars: map[string]float64{
				"seconds":           float64(req.Seconds),
				"n":                 float64(req.N),
				"task_status":       1, // pre-charge assumes success
				"prompt_tokens":     float64(promptEst),
				"completion_tokens": 0,
				"total_tokens":      float64(promptEst),
			},
		}
	}
	price, set := options.ModelPriceUSD, options.ModelPriceSet
	if !set && strings.HasPrefix(modelName, "sora") {
		price, set = soraPricePerSecond, true
	}
	if set && price > 0 {
		return channel.TaskPricing{
			PerCallUSD: price,
			Units:      float64(req.Seconds) * float64(req.N),
		}
	}
	return channel.TaskPricing{
		PromptTokens:     dto.EstimateTokens(req.Prompt) + 100,
		CompletionTokens: 0,
	}
}

func (VideoTaskAdaptor) AdjustBillingOnComplete(modelName, group string, req *dto.TaskRequest, res *dto.TaskResult, accepted channel.TaskPricing) channel.TaskPricing {
	// Expression path: update ExprVars with the terminal status. The
	// expression handles status-based discounting via the task_status
	// variable (success=1, cancelled=0.5, failed=0), so the adaptor no
	// longer hardcodes Units *= 0.5 for cancellations. Seconds/n stay
	// from the request (the platform does not report delivered values).
	if len(accepted.ExprVars) > 0 {
		accepted.ExprVars["task_status"] = taskStatusValue(res.Status)
		return accepted
	}
	// Non-expression path: preserve existing per-call / token fallback.
	switch res.Status {
	case dto.TaskStatusSucceeded:
		// The platform does not report delivered seconds; the request
		// parameters remain the bill of record.
		return accepted
	case dto.TaskStatusCancelled:
		// Cancellations are refunded at half rate (work already spent).
		if accepted.PerCallUSD > 0 {
			accepted.Units *= 0.5
			return accepted
		}
		accepted.PromptTokens /= 2
		accepted.CompletionTokens /= 2
		return accepted
	default:
		// Failed tasks are fully refunded.
		return channel.TaskPricing{}
	}
}

// taskStatusValue maps a terminal task status to the numeric value used by
// pricing expressions: succeeded=1, cancelled=0.5, failed/unknown=0.
func taskStatusValue(status string) float64 {
	switch status {
	case dto.TaskStatusSucceeded:
		return 1
	case dto.TaskStatusCancelled:
		return 0.5
	default:
		return 0
	}
}

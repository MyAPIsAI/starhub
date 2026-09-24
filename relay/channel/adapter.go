// Package channel defines the per-provider adaptor contracts (P3.1).
// Every upstream provider lives in its own package and registers in
// relay/relay_adaptor.go; adding a provider means implementing ChatAdaptor
// (or TaskAdaptor) and adding one switch case — the relay core never
// changes.
package channel

import (
	"errors"
	"net/http"

	"starhub/relay/dto"
)

// Channel type identifiers. The numbering of the P3 providers mirrors the
// new-api convention so channel configurations port over unchanged.
const (
	ChannelTypeOpenAI = 1
	ChannelTypeClaude = 14
	ChannelTypeAWS    = 15
	ChannelTypeGemini = 25
)

// RelayMode selects the endpoint family of one relay request.
type RelayMode int

const (
	RelayModeChat RelayMode = iota
	RelayModeEmbeddings
	RelayModeRerank
	RelayModeTTS
	RelayModeSTT
	RelayModeSTTTranslation
	RelayModeImageGeneration
	RelayModeImageEdit
)

// ErrUnsupportedMode is returned by adaptors that do not serve a relay mode;
// the retry loop skips such channels instead of failing the request.
var ErrUnsupportedMode = errors.New("channel type does not support this relay mode")

// ChatStreamReader pulls IR stream events from an upstream SSE response.
// Next returns (nil, io.EOF) once the upstream stream ended.
type ChatStreamReader interface {
	Next() (*dto.ChatStreamEvent, error)
	Close() error
}

// ChatAdaptor is the contract of a chat-capable upstream provider:
//
//	Init/auth          Name / ChatURL / ModeURL / PrepareRequest
//	protocol convert   ConvertChatRequest (IR -> provider wire)
//	passthrough        NativeInboundFormat / PreparePassthroughBody
//	response parse     HandlePassthroughResponse / ParseChatResponse /
//	                   StreamChatEvents
type ChatAdaptor interface {
	Name() string
	// NativeInboundFormat reports the inbound protocol the provider serves
	// natively (byte-level passthrough); "" means every inbound protocol is
	// served through IR conversion.
	NativeInboundFormat() dto.InboundFormat
	// ChatURL builds the chat completion endpoint of the provider. It
	// receives the channel key because signed providers derive the
	// endpoint from credentials (AWS regions).
	ChatURL(baseURL, apiKey, model string, stream bool) string
	// ModeURL builds the endpoint of a non-chat relay mode (embeddings,
	// rerank, audio, images). ErrUnsupportedMode skips the channel.
	ModeURL(baseURL string, mode RelayMode) (string, error)
	// RealtimeURL builds the websocket endpoint of the realtime API.
	// ErrUnsupportedMode skips the channel.
	RealtimeURL(baseURL, model string) (string, error)
	// PrepareRequest finalizes the upstream request: auth headers and, for
	// signed providers (AWS SigV4), the request signature. It receives the
	// exact body bytes that will be sent.
	PrepareRequest(req *http.Request, apiKey string, body []byte) error
	// PreparePassthroughBody adapts a native-format inbound body (the only
	// mutation today is the OpenAI stream_options injection).
	PreparePassthroughBody(body []byte, stream bool) ([]byte, error)
	// ConvertChatRequest serializes IR into the provider wire format.
	ConvertChatRequest(ir *dto.ChatRequest) ([]byte, error)
	// HandlePassthroughResponse relays a native-format response to the
	// client and extracts usage/content for settlement.
	HandlePassthroughResponse(w http.ResponseWriter, resp *http.Response, stream bool) (usage *dto.Usage, content string, err error)
	// ParseChatResponse reads a non-stream response into IR.
	ParseChatResponse(resp *http.Response) (*dto.ChatResponse, error)
	// StreamChatEvents wraps an SSE response into IR events.
	StreamChatEvents(resp *http.Response) (ChatStreamReader, error)
}

// TaskPricing is the provider-agnostic bill of one task (P3.3). The relay
// billing core converts it into quota; platforms never touch the quota
// math themselves.
type TaskPricing struct {
	// PerCallUSD > 0 bills PerCallUSD x Units (per-call priced models).
	PerCallUSD float64
	Units      float64
	// PromptTokens / CompletionTokens price token-billed models.
	PromptTokens     int64
	CompletionTokens int64
}

// TaskAdaptor is the submit-and-poll lifecycle of asynchronous task
// platforms (video generation, music, ...). The billing hooks
// (EstimateBilling / AdjustBillingOnComplete) are part of the contract so
// every platform plugs into the same pre-charge / terminal-settlement
// flow instead of reimplementing it.
type TaskAdaptor interface {
	Name() string
	Platform() string
	// ValidateRequestAndSetAction parses and validates the submit body and
	// fills Action/Model on the normalized request.
	ValidateRequestAndSetAction(body []byte, req *dto.TaskRequest) error
	// EstimateBilling prices the submission (pre-charge input).
	EstimateBilling(modelName, group string, req *dto.TaskRequest) TaskPricing
	// AdjustBillingOnComplete prices the terminal task (settlement input;
	// a failed task prices to zero and is fully refunded).
	AdjustBillingOnComplete(modelName, group string, req *dto.TaskRequest, res *dto.TaskResult, accepted TaskPricing) TaskPricing
	// Upstream plumbing.
	SubmitURL(baseURL string) string
	FetchURL(baseURL, taskID string) string
	PrepareRequest(req *http.Request, apiKey string, body []byte) error
	ConvertSubmitRequest(req *dto.TaskRequest) ([]byte, error)
	ParseSubmitResponse(statusCode int, body []byte) (taskID string, err error)
	ParseTaskResult(statusCode int, body []byte) (*dto.TaskResult, error)
}

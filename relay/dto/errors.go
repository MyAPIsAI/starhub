package dto

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// ErrorEnvelope is the normalized error the relay shows clients. Upstream
// errors arrive in OpenAI / Claude / Gemini shapes; the relay converts the
// envelope into the client's inbound format so a Claude SDK talking to an
// OpenAI channel still receives Claude-shaped errors (P3.2 matrix).
type ErrorEnvelope struct {
	Type    string // machine type, e.g. "invalid_request_error"
	Code    string // provider error code when present
	Message string
}

// ParseProviderErrorBody extracts an ErrorEnvelope from an upstream error
// body by trying the known provider shapes. A nil result means the body did
// not look like a structured error (the raw body is then used verbatim).
func ParseProviderErrorBody(body []byte) *ErrorEnvelope {
	// OpenAI / Gemini style: {"error":{"message","type","code"}}
	var openai struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    any    `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &openai); err == nil && openai.Error.Message != "" {
		env := &ErrorEnvelope{
			Message: openai.Error.Message,
			Type:    orDefault(openai.Error.Type, "api_error"),
			Code:    fmt.Sprintf("%v", openai.Error.Code),
		}
		if env.Code == "<nil>" {
			env.Code = ""
		}
		return env
	}
	// Claude style: {"type":"error","error":{"type","message"}}
	var claude struct {
		Type  string `json:"type"`
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &claude); err == nil && claude.Error.Message != "" {
		return &ErrorEnvelope{
			Message: claude.Error.Message,
			Type:    orDefault(claude.Error.Type, "api_error"),
		}
	}
	return nil
}

// BuildOpenAIErrorBody renders the OpenAI error envelope.
func BuildOpenAIErrorBody(env *ErrorEnvelope) []byte {
	body := map[string]any{
		"error": map[string]any{
			"message": env.Message,
			"type":    orDefault(env.Type, "api_error"),
			"code":    env.Code,
		},
	}
	raw, _ := json.Marshal(body)
	return raw
}

// BuildClaudeErrorBody renders the Claude error envelope.
func BuildClaudeErrorBody(env *ErrorEnvelope) []byte {
	body := map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    orDefault(env.Type, "api_error"),
			"message": env.Message,
		},
	}
	raw, _ := json.Marshal(body)
	return raw
}

// WriteRelayError writes a gateway-generated error in the inbound format.
func WriteRelayError(w http.ResponseWriter, status int, inbound InboundFormat, errType, code, message string) {
	if inbound == InboundGemini {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(BuildGeminiErrorBody(status, message))
		return
	}
	if inbound == InboundClaude {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(BuildClaudeErrorBody(&ErrorEnvelope{Type: errType, Message: message}))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(BuildOpenAIErrorBody(&ErrorEnvelope{Type: errType, Code: code, Message: message}))
}

// WriteUpstreamError passes an upstream error body through to the client,
// converting the envelope when the inbound format differs from the upstream's.
func WriteUpstreamError(w http.ResponseWriter, status int, inbound InboundFormat, upstreamBody []byte, upstreamContentType string) {
	if inbound == InboundGemini && json.Valid(upstreamBody) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(upstreamBody)
		return
	}
	env := ParseProviderErrorBody(upstreamBody)
	if env == nil {
		// Not a structured provider error: forward the bytes unchanged.
		ct := upstreamContentType
		if ct == "" {
			ct = "application/json"
		}
		w.Header().Set("Content-Type", ct)
		w.WriteHeader(status)
		_, _ = w.Write(upstreamBody)
		return
	}
	if inbound == InboundClaude {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(BuildClaudeErrorBody(env))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(BuildOpenAIErrorBody(env))
}

// BuildGeminiErrorBody follows the native Google RPC JSON error shape.
func BuildGeminiErrorBody(status int, message string) []byte {
	name := map[int]string{400: "INVALID_ARGUMENT", 401: "UNAUTHENTICATED", 403: "PERMISSION_DENIED", 404: "NOT_FOUND", 409: "ABORTED", 413: "RESOURCE_EXHAUSTED", 429: "RESOURCE_EXHAUSTED", 500: "INTERNAL", 502: "UNAVAILABLE", 503: "UNAVAILABLE"}[status]
	if name == "" {
		name = "UNKNOWN"
	}
	body, _ := json.Marshal(map[string]any{"error": map[string]any{"code": status, "message": message, "status": name}})
	return body
}

package relay

import (
	"encoding/json"
	"fmt"
	"net/http"

	"starhub/relay/dto"
)

// WriteChatResponse serializes a non-stream IR chat response in the
// client's inbound format (P3.2 matrix: any inbound x any channel).
func WriteChatResponse(w http.ResponseWriter, inbound dto.InboundFormat, resp *dto.ChatResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if inbound == dto.InboundClaude {
		body, err := dto.BuildClaudeChatResponse(resp)
		if err != nil {
			return
		}
		_, _ = w.Write(body)
		return
	}
	id := resp.ID
	if id == "" {
		id = "chatcmpl-starhub"
	}
	finish := resp.FinishReason
	if finish == "" {
		finish = "stop"
	}
	message := map[string]any{"role": "assistant", "content": resp.Text}
	if len(resp.ToolCalls) > 0 {
		calls := make([]map[string]any, 0, len(resp.ToolCalls))
		for _, tc := range resp.ToolCalls {
			calls = append(calls, map[string]any{
				"id":   tc.ID,
				"type": "function",
				"function": map[string]any{
					"name":      tc.Name,
					"arguments": tc.Arguments,
				},
			})
		}
		message["tool_calls"] = calls
		if finish == "stop" {
			finish = "tool_calls"
		}
	}
	body := map[string]any{
		"id":      id,
		"object":  "chat.completion",
		"created": 0,
		"model":   resp.Model,
		"choices": []map[string]any{{
			"index":         0,
			"message":       message,
			"finish_reason": finish,
		}},
	}
	if resp.Usage != nil {
		body["usage"] = resp.Usage
	}
	raw, _ := json.Marshal(body)
	_, _ = w.Write(raw)
}

// ChatStreamEmitter serializes IR stream events in the client's inbound
// format. Events are buffered per protocol frame and flushed immediately.
type ChatStreamEmitter interface {
	// Emit writes one delta; it returns an error once the client is gone.
	Emit(ev *dto.ChatStreamEvent) error
	// Finish writes the terminal frames (usage + [DONE] / message_stop).
	Finish(usage *Usage, finishReason string) error
}

// NewChatStreamEmitter creates the emitter of an inbound format.
func NewChatStreamEmitter(w http.ResponseWriter, inbound dto.InboundFormat, model string) ChatStreamEmitter {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	if inbound == dto.InboundClaude {
		return &claudeStreamEmitter{w: w, model: model, usage: &Usage{}}
	}
	return &openAIStreamEmitter{w: w, model: model}
}

type openAIStreamEmitter struct {
	w       http.ResponseWriter
	model   string
	started bool
}

func (e *openAIStreamEmitter) Emit(ev *dto.ChatStreamEvent) error {
	delta := map[string]any{}
	if !e.started {
		delta["role"] = "assistant"
		e.started = true
	}
	if ev.TextDelta != "" {
		delta["content"] = ev.TextDelta
	}
	if len(delta) == 0 {
		return nil // finish-only events carry no client-visible delta
	}
	chunk := map[string]any{
		"id":      "chatcmpl-starhub",
		"object":  "chat.completion.chunk",
		"created": 0,
		"model":   e.model,
		"choices": []map[string]any{{
			"index":         0,
			"delta":         delta,
			"finish_reason": nil,
		}},
	}
	return writeSSE(e.w, "data: "+mustMarshalString(chunk))
}

func (e *openAIStreamEmitter) Finish(usage *Usage, finishReason string) error {
	if finishReason == "" {
		finishReason = "stop"
	}
	chunk := map[string]any{
		"id":      "chatcmpl-starhub",
		"object":  "chat.completion.chunk",
		"created": 0,
		"model":   e.model,
		"choices": []map[string]any{{
			"index":         0,
			"delta":         map[string]any{},
			"finish_reason": finishReason,
		}},
	}
	if usage != nil {
		chunk["usage"] = usage
	}
	if err := writeSSE(e.w, "data: "+mustMarshalString(chunk)); err != nil {
		return err
	}
	return writeSSE(e.w, "data: [DONE]")
}

type claudeStreamEmitter struct {
	w         http.ResponseWriter
	model     string
	usage     *Usage
	blockOpen bool
	started   bool
}

func (e *claudeStreamEmitter) Emit(ev *dto.ChatStreamEvent) error {
	if ev.Usage != nil {
		e.usage = ev.Usage.Clone()
	}
	if !e.started {
		e.started = true
		usage := dto.ClaudeUsageFromIR(e.usage)
		usage.OutputTokens = 0
		message := map[string]any{
			"id":    "msg_starhub",
			"type":  "message",
			"role":  "assistant",
			"model": e.model,
			"usage": usage,
		}
		if err := writeSSE(e.w, "event: message_start\ndata: "+mustMarshalString(map[string]any{
			"type": "message_start", "message": message,
		})); err != nil {
			return err
		}
	}
	if ev.TextDelta == "" {
		return nil
	}
	if !e.blockOpen {
		e.blockOpen = true
		if err := writeSSE(e.w, "event: content_block_start\ndata: "+mustMarshalString(map[string]any{
			"type": "content_block_start", "index": 0,
			"content_block": map[string]any{"type": "text", "text": ""},
		})); err != nil {
			return err
		}
	}
	return writeSSE(e.w, "event: content_block_delta\ndata: "+mustMarshalString(map[string]any{
		"type": "content_block_delta", "index": 0,
		"delta": map[string]any{"type": "text_delta", "text": ev.TextDelta},
	}))
}

func (e *claudeStreamEmitter) Finish(usage *Usage, finishReason string) error {
	if usage != nil {
		e.usage = usage.Clone()
	}
	if !e.started {
		// Empty upstream stream: still emit a well-formed (empty) message.
		if err := e.Emit(&dto.ChatStreamEvent{}); err != nil {
			return err
		}
	}
	if e.blockOpen {
		if err := writeSSE(e.w, "event: content_block_stop\ndata: "+mustMarshalString(map[string]any{
			"type": "content_block_stop", "index": 0,
		})); err != nil {
			return err
		}
	}
	stopReason := claudeStopReason(finishReason)
	if err := writeSSE(e.w, "event: message_delta\ndata: "+mustMarshalString(map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": stopReason, "stop_sequence": nil},
		"usage": dto.ClaudeUsageFromIR(e.usage),
	})); err != nil {
		return err
	}
	return writeSSE(e.w, "event: message_stop\ndata: "+mustMarshalString(map[string]any{
		"type": "message_stop",
	}))
}

func claudeStopReason(finish string) string {
	switch finish {
	case "", "stop":
		return "end_turn"
	case "length":
		return "max_tokens"
	case "tool_calls":
		return "tool_use"
	}
	return finish
}

func writeSSE(w http.ResponseWriter, frame string) error {
	if _, err := fmt.Fprintf(w, "%s\n\n", frame); err != nil {
		return err
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}

func mustMarshalString(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

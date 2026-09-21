package dto

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOpenAIRoundTripToClaude(t *testing.T) {
	src := `{
		"model": "claude-3-5-sonnet",
		"messages": [
			{"role": "system", "content": "be brief"},
			{"role": "user", "content": [
				{"type": "text", "text": "describe this"},
				{"type": "image_url", "image_url": {"url": "data:image/png;base64,AAEC"}}
			]},
			{"role": "assistant", "content": "sure", "tool_calls": [
				{"id": "call_1", "type": "function", "function": {"name": "lookup", "arguments": "{\"q\":\"cat\"}"}}
			]},
			{"role": "tool", "tool_call_id": "call_1", "content": "a furry animal"}
		],
		"max_tokens": 512,
		"temperature": 0.7,
		"stop": ["END"],
		"tools": [{"type": "function", "function": {"name": "lookup", "description": "search", "parameters": {"type": "object"}}}]
	}`
	ir, err := ParseOpenAIChatRequest([]byte(src))
	if err != nil {
		t.Fatalf("parse openai: %v", err)
	}
	if len(ir.Messages) != 4 || ir.MaxTokens != 512 || len(ir.Tools) != 1 {
		t.Fatalf("IR wrong: %+v", ir)
	}
	if ir.Messages[1].Images[0].Base64 != "AAEC" || ir.Messages[1].Images[0].MediaType != "image/png" {
		t.Fatalf("image part not parsed: %+v", ir.Messages[1].Images)
	}
	if ir.Messages[2].ToolCalls[0].Name != "lookup" || ir.Messages[3].ToolCallID != "call_1" {
		t.Fatalf("tool conversion wrong: %+v", ir.Messages[2:])
	}

	claudeBody, err := BuildClaudeChatRequest(ir)
	if err != nil {
		t.Fatalf("build claude: %v", err)
	}
	var claudeReq struct {
		System    json.RawMessage   `json:"system"`
		Messages  []json.RawMessage `json:"messages"`
		MaxTokens int64             `json:"max_tokens"`
		Tools     []map[string]any  `json:"tools"`
		Stop      []string          `json:"stop_sequences"`
	}
	if err := json.Unmarshal(claudeBody, &claudeReq); err != nil {
		t.Fatalf("unmarshal claude: %v (%s)", err, claudeBody)
	}
	if string(claudeReq.System) != `"be brief"` {
		t.Fatalf("system wrong: %s", claudeReq.System)
	}
	if claudeReq.MaxTokens != 512 || len(claudeReq.Tools) != 1 || claudeReq.Tools[0]["name"] != "lookup" {
		t.Fatalf("claude fields wrong: %s", claudeBody)
	}
	// system became the first message? no: it must be REMOVED from messages
	if len(claudeReq.Messages) != 3 {
		t.Fatalf("claude must merge system out of messages: %s", claudeBody)
	}
	// the tool result rides on a user turn with a tool_result block
	var lastMsg map[string]any
	if err := json.Unmarshal(claudeReq.Messages[2], &lastMsg); err != nil {
		t.Fatalf("bad message: %v", err)
	}
	if lastMsg["role"] != "user" {
		t.Fatalf("tool result must be a claude user turn, got %v", lastMsg["role"])
	}
	blocks, _ := lastMsg["content"].([]any)
	if len(blocks) == 0 {
		t.Fatalf("tool result content missing: %s", claudeBody)
	}
	block, _ := blocks[0].(map[string]any)
	if block["type"] != "tool_result" || block["tool_use_id"] != "call_1" {
		t.Fatalf("tool_result block wrong: %v", block)
	}

	// Back into IR: the claude body must survive a re-parse.
	ir2, err := ParseClaudeChatRequest(claudeBody)
	if err != nil {
		t.Fatalf("re-parse claude: %v", err)
	}
	if ir2.PromptText() != ir.PromptText() {
		t.Fatalf("prompt text drifted: %q vs %q", ir2.PromptText(), ir.PromptText())
	}
	if ir2.MaxTokens != ir.MaxTokens {
		t.Fatalf("max tokens drifted: %d vs %d", ir2.MaxTokens, ir.MaxTokens)
	}
}

func TestParseClaudeChatResponse(t *testing.T) {
	body := `{
		"id": "msg_01",
		"model": "claude-3-5-sonnet",
		"content": [
			{"type": "text", "text": "hello "},
			{"type": "text", "text": "world"},
			{"type": "tool_use", "id": "toolu_01", "name": "lookup", "input": {"q": "cat"}}
		],
		"stop_reason": "tool_use",
		"usage": {"input_tokens": 25, "output_tokens": 40}
	}`
	resp, err := ParseClaudeChatResponse([]byte(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if resp.Text != "hello world" {
		t.Fatalf("text = %q", resp.Text)
	}
	if resp.FinishReason != "tool_calls" {
		t.Fatalf("finish = %q", resp.FinishReason)
	}
	if resp.Usage.PromptTokens != 25 || resp.Usage.CompletionTokens != 40 {
		t.Fatalf("usage wrong: %+v", resp.Usage)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].ID != "toolu_01" {
		t.Fatalf("tool calls wrong: %+v", resp.ToolCalls)
	}

	// IR -> claude response for claude-in clients.
	out, err := BuildClaudeChatResponse(resp)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.Contains(string(out), `"type":"message"`) ||
		!strings.Contains(string(out), `"input_tokens":25`) ||
		!strings.Contains(string(out), `"tool_use"`) {
		t.Fatalf("claude response wrong: %s", out)
	}
}

func TestClaudeStreamEvents(t *testing.T) {
	start := `{"type":"message_start","message":{"id":"msg_1","model":"m","usage":{"input_tokens":10,"output_tokens":1}}}`
	ev, err := ParseClaudeStreamEvent([]byte(start))
	if err != nil || ev.Type != "message_start" || ev.Usage.PromptTokens != 10 {
		t.Fatalf("message_start parse: %+v %v", ev, err)
	}
	delta := `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}`
	ev, err = ParseClaudeStreamEvent([]byte(delta))
	if err != nil || ev.TextDelta != "hi" {
		t.Fatalf("delta parse: %+v %v", ev, err)
	}
	final := `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`
	ev, err = ParseClaudeStreamEvent([]byte(final))
	if err != nil || ev.StopReason != "end_turn" || ev.Usage.CompletionTokens != 5 {
		t.Fatalf("message_delta parse: %+v %v", ev, err)
	}
}

func TestGeminiConversion(t *testing.T) {
	src := `{
		"model": "gemini-1.5-flash",
		"messages": [
			{"role": "system", "content": "sys"},
			{"role": "user", "content": "hello"},
			{"role": "assistant", "content": "hi"},
			{"role": "user", "content": "and now?"}
		],
		"max_tokens": 128,
		"temperature": 0.2
	}`
	ir, err := ParseOpenAIChatRequest([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	body, err := BuildGeminiChatRequest(ir)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	var req struct {
		SystemInstruction *struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"systemInstruction"`
		Contents []struct {
			Role  string `json:"role"`
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"contents"`
		GenerationConfig struct {
			MaxOutputTokens int64    `json:"maxOutputTokens"`
			Temperature     *float64 `json:"temperature"`
		} `json:"generationConfig"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, body)
	}
	if req.SystemInstruction == nil || req.SystemInstruction.Parts[0].Text != "sys" {
		t.Fatalf("system instruction wrong: %s", body)
	}
	if len(req.Contents) != 3 || req.Contents[1].Role != "model" {
		t.Fatalf("contents wrong: %s", body)
	}
	if req.GenerationConfig.MaxOutputTokens != 128 {
		t.Fatalf("config wrong: %s", body)
	}

	// Gemini response -> IR
	respBody := `{"candidates":[{"content":{"parts":[{"text":"answer"}]},"finishReason":"STOP"}],
		"usageMetadata":{"promptTokenCount":12,"candidatesTokenCount":7}}`
	resp, err := ParseGeminiChatResponse([]byte(respBody))
	if err != nil {
		t.Fatalf("parse resp: %v", err)
	}
	if resp.Text != "answer" || resp.FinishReason != "stop" ||
		resp.Usage.PromptTokens != 12 || resp.Usage.CompletionTokens != 7 {
		t.Fatalf("resp IR wrong: %+v", resp)
	}

	// Gemini stream chunk -> IR event (usage only on later chunks).
	chunk := `{
	  "candidates": [
	    {"content": {"parts": [{"text": "par"}]}}
	  ]
	}`
	ev, err := ParseGeminiStreamPayload([]byte(chunk))
	if err != nil || ev.TextDelta != "par" {
		t.Fatalf("stream event wrong: %+v %v", ev, err)
	}
}

func TestBedrockConversion(t *testing.T) {
	ir := &ChatRequest{
		Model: "anthropic.claude-3-5-sonnet",
		Messages: []ChatMessage{
			{Role: "system", Text: "sys"},
			{Role: "user", Text: "q"},
		},
		MaxTokens: 64,
	}
	body, err := BuildBedrockChatRequest(ir)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.Contains(string(body), `"system":[{"text":"sys"}]`) ||
		!strings.Contains(string(body), `"role":"user"`) ||
		!strings.Contains(string(body), `"maxTokens":64`) {
		t.Fatalf("bedrock body wrong: %s", body)
	}

	respBody := `{"output":{"message":{"role":"assistant","content":[{"text":"a"}]}},
		"stopReason":"end_turn","usage":{"inputTokens":9,"outputTokens":4}}`
	resp, err := ParseBedrockChatResponse([]byte(respBody))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if resp.Text != "a" || resp.FinishReason != "stop" ||
		resp.Usage.PromptTokens != 9 || resp.Usage.CompletionTokens != 4 {
		t.Fatalf("bedrock resp IR wrong: %+v", resp)
	}
}

func TestBedrockStreamEvent(t *testing.T) {
	ev, err := ParseBedrockStreamEvent([]byte(`{"type":"contentBlockDelta","delta":{"text":"x"}}`))
	if err != nil || ev.TextDelta != "x" {
		t.Fatalf("delta: %+v %v", ev, err)
	}
	ev, err = ParseBedrockStreamEvent([]byte(`{"type":"metadata","usage":{"inputTokens":10,"outputTokens":20}}`))
	if err != nil || ev.Usage.PromptTokens != 10 || ev.Usage.CompletionTokens != 20 {
		t.Fatalf("metadata: %+v %v", ev, err)
	}
}

func TestImageRequestValidation(t *testing.T) {
	// unknown fields survive in Extra and re-serialize
	body := `{"model":"gpt-image-1","prompt":"a cat","n":3,"size":"1024x1024",
		"quality":"high","zhipu_style":"anime","watermark":false}`
	req, err := ParseImageRequest([]byte(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if req.Count() != 3 || req.Size != "1024x1024" {
		t.Fatalf("fields wrong: %+v", req)
	}
	if _, ok := req.Extra["zhipu_style"]; !ok {
		t.Fatalf("unknown fields must survive: %+v", req.Extra)
	}
	out, err := req.MarshalBody()
	if err != nil || !strings.Contains(string(out), `"zhipu_style":"anime"`) {
		t.Fatalf("extra fields must re-serialize: %s %v", out, err)
	}

	// n bounds (wrap-around included)
	for _, raw := range []string{
		`{"model":"m","prompt":"p","n":0}`,
		`{"model":"m","prompt":"p","n":129}`,
	} {
		if _, err := ParseImageRequest([]byte(raw)); err == nil {
			t.Fatalf("n bound must reject %s", raw)
		}
	}
	// full-width multiplication sign gets its own hint
	_, err = ParseImageRequest([]byte(`{"model":"m","prompt":"p","size":"1024×1024"}`))
	if err == nil || !strings.Contains(err.Error(), `"x"`) {
		t.Fatalf("full-width size must be rejected with a hint: %v", err)
	}
}

func TestImageStreamEventParse(t *testing.T) {
	ev, err := ParseImageStreamEvent(nil, []byte(
		`{"type":"image_generation.completed","output_index":0,"b64_json":"QUJD"}`))
	if err != nil || ev.Type != "image_generation.completed" {
		t.Fatalf("completed: %+v %v", ev, err)
	}
	ev, err = ParseImageStreamEvent(nil, []byte(
		`{"type":"error","error":{"code":"x","message":"boom"}}`))
	if err != nil || ev.Type != "error" || ev.ErrorMessage != "boom" {
		t.Fatalf("error event: %+v %v", ev, err)
	}
}

func TestEmbeddingAndRerankValidation(t *testing.T) {
	if _, err := ParseEmbeddingRequest([]byte(`{"model":"e","input":["a","b"]}`)); err != nil {
		t.Fatalf("array input must parse: %v", err)
	}
	if _, err := ParseEmbeddingRequest([]byte(`{"model":"e","input":"s"}`)); err != nil {
		t.Fatalf("string input must parse: %v", err)
	}
	if _, err := ParseEmbeddingRequest([]byte(`{"model":"e"}`)); err == nil {
		t.Fatal("missing input must fail")
	}
	many := `{"model":"e","input":[`
	for i := 0; i <= MaxEmbeddingInputs; i++ {
		many += `"x",`
	}
	many += `"y"]}`
	if _, err := ParseEmbeddingRequest([]byte(many)); err == nil {
		t.Fatal("input item bound must hold")
	}

	if _, err := ParseRerankRequest([]byte(`{"model":"r","query":"q","documents":["d1","d2"]}`)); err != nil {
		t.Fatalf("rerank parse: %v", err)
	}
	if _, err := ParseRerankRequest([]byte(`{"model":"r","query":"q"}`)); err == nil {
		t.Fatal("missing documents must fail")
	}
}

func TestRerankUsageShapes(t *testing.T) {
	for _, body := range []string{
		`{"usage":{"input_tokens":10,"output_tokens":2}}`,
		`{"usage":{"billed_tokens":7}}`,
		`{"usage":{"billing_tokens":8}}`,
	} {
		usage, err := ParseRerankUsage([]byte(body))
		if err != nil || usage.PromptTokens != 10 && usage.PromptTokens != 7 && usage.PromptTokens != 8 {
			t.Fatalf("usage shape %s: %+v %v", body, usage, err)
		}
	}
}

func TestTTSValidation(t *testing.T) {
	if _, err := ParseTTSRequest([]byte(`{"model":"tts-1","input":"hi","voice":"alloy"}`)); err != nil {
		t.Fatalf("parse: %v", err)
	}
	long := strings.Repeat("x", MaxTTSInputChars+1)
	if _, err := ParseTTSRequest([]byte(`{"model":"tts-1","input":"` + long + `"}`)); err == nil {
		t.Fatal("input length bound must hold")
	}
}

func TestVideoTaskValidation(t *testing.T) {
	if _, err := ParseVideoTaskRequest([]byte(`{"model":"sora-2","prompt":"a cat","seconds":8,"n":2}`)); err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, raw := range []string{
		`{"model":"sora-2","prompt":"p","seconds":0}`,
		`{"model":"sora-2","prompt":"p","seconds":61}`,
		`{"model":"sora-2","prompt":"p","n":11}`,
		`{"model":"sora-2","prompt":"p","n":0}`,
		`{"prompt":"p"}`,
		`{"model":"sora-2"}`,
	} {
		if _, err := ParseVideoTaskRequest([]byte(raw)); err == nil {
			t.Fatalf("bounds must reject %s", raw)
		}
	}
	// defaults mirror the platform defaults
	req, err := ParseVideoTaskRequest([]byte(`{"model":"sora-2","prompt":"p"}`))
	if err != nil || req.Seconds != 4 || req.N != 1 {
		t.Fatalf("defaults wrong: %+v", req)
	}
}

func TestVideoTaskResultNormalization(t *testing.T) {
	res, err := ParseVideoTaskResult([]byte(
		`{"id":"v1","status":"completed","progress":100,"video":{"url":"https://cdn/v1.mp4"}}`))
	if err != nil || res.Status != TaskStatusSucceeded || res.Outputs[0] != "https://cdn/v1.mp4" {
		t.Fatalf("completed: %+v %v", res, err)
	}
	res, err = ParseVideoTaskResult([]byte(
		`{"id":"v2","status":"failed","error":{"code":"content_policy","message":"nope"}}`))
	if err != nil || res.Status != TaskStatusFailed || res.FailReason != "nope" {
		t.Fatalf("failed: %+v %v", res, err)
	}
}

func TestErrorEnvelopeMatrix(t *testing.T) {
	// OpenAI error -> Claude envelope
	env := ParseProviderErrorBody([]byte(
		`{"error":{"message":"bad key","type":"invalid_request_error","code":"invalid_api_key"}}`))
	if env == nil || env.Message != "bad key" || env.Code != "invalid_api_key" {
		t.Fatalf("envelope: %+v", env)
	}
	claudeBody := BuildClaudeErrorBody(env)
	if !strings.Contains(string(claudeBody), `"type":"error"`) ||
		!strings.Contains(string(claudeBody), `"message":"bad key"`) {
		t.Fatalf("claude error body: %s", claudeBody)
	}
	// Claude error -> OpenAI envelope
	env = ParseProviderErrorBody([]byte(`{"type":"error","error":{"type":"overloaded_error","message":"nope"}}`))
	if env == nil || env.Message != "nope" {
		t.Fatalf("claude envelope: %+v", env)
	}
	openaiBody := BuildOpenAIErrorBody(env)
	if !strings.Contains(string(openaiBody), `"error"`) ||
		!strings.Contains(string(openaiBody), `"message":"nope"`) {
		t.Fatalf("openai error body: %s", openaiBody)
	}
	// garbage stays raw (nil)
	if ParseProviderErrorBody([]byte(`not json`)) != nil {
		t.Fatal("unparseable body must return nil")
	}
}

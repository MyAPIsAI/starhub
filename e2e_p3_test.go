package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
	"starhub/relay"
	"starhub/relay/channel/aws"
	"starhub/service"
)

// ---------------------------------------------------------------------------
// P3.1 chat matrix: mock upstreams speaking each provider protocol
// ---------------------------------------------------------------------------

const (
	claudeUpstreamKey = "sk-claude-upstream-key"
	geminiUpstreamKey = "gemini-upstream-key"
	bedrockAccessKey  = "AKIAIOSFODNN7EXAMPLE"
	bedrockSecretKey  = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	bedrockRegion     = "us-east-1"
)

var sigV4Pattern = regexp.MustCompile(`^AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/[0-9]{8}/us-east-1/bedrock/aws4_request, SignedHeaders=[a-z0-9;-]+, Signature=[0-9a-f]{64}$`)

// newClaudeUpstream mocks the Anthropic /v1/messages API and records what
// it received.
func newClaudeUpstream(t *testing.T) (*httptest.Server, *atomic.Value) {
	t.Helper()
	var lastBody atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("x-api-key") != claudeUpstreamKey {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`)
			return
		}
		if r.Header.Get("anthropic-version") == "" {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"type":"error","error":{"type":"invalid_request_error","message":"anthropic-version header is required"}}`)
			return
		}
		body, _ := io.ReadAll(r.Body)
		lastBody.Store(string(body))
		var req struct {
			Model     string `json:"model"`
			System    string `json:"system"`
			Stream    bool   `json:"stream"`
			MaxTokens int64  `json:"max_tokens"`
			Messages  []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if req.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			flusher := w.(http.Flusher)
			events := []string{
				`event: message_start` + "\n" + `data: {"type":"message_start","message":{"id":"msg_1","role":"assistant","model":"` + req.Model + `","usage":{"input_tokens":100,"output_tokens":1}}}`,
				`event: content_block_start` + "\n" + `data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
				`event: content_block_delta` + "\n" + `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello "}}`,
				`event: content_block_delta` + "\n" + `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"from claude"}}`,
				`event: content_block_stop` + "\n" + `data: {"type":"content_block_stop","index":0}`,
				`event: message_delta` + "\n" + `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":50}}`,
				`event: message_stop` + "\n" + `data: {"type":"message_stop"}`,
			}
			for _, ev := range events {
				fmt.Fprintf(w, "%s\n\n", ev)
				flusher.Flush()
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"id":"msg_1","type":"message","role":"assistant","model":%q,"content":[{"type":"text","text":"hello from claude"}],"stop_reason":"end_turn","usage":{"input_tokens":100,"output_tokens":50}}`, req.Model)
	}))
	t.Cleanup(srv.Close)
	return srv, &lastBody
}

// newGeminiUpstream mocks the Gemini generateContent API.
func newGeminiUpstream(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-goog-api-key") != geminiUpstreamKey {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"error":{"code":401,"message":"API key not valid","status":"UNAUTHENTICATED"}}`)
			return
		}
		stream := strings.Contains(r.URL.Path, ":streamGenerateContent")
		if !strings.Contains(r.URL.Path, "/v1beta/models/gemini-1.5-flash") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if stream {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			flusher := w.(http.Flusher)
			for _, data := range []string{
				`{"candidates":[{"content":{"parts":[{"text":"hello "}]}}]}`,
				`{"candidates":[{"content":{"parts":[{"text":"from gemini"}]}}],"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":50,"totalTokenCount":150}}`,
			} {
				fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"candidates":[{"content":{"parts":[{"text":"hello from gemini"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":50,"totalTokenCount":150}}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// newBedrockUpstream mocks the Bedrock converse API (invoke + eventstream).
func newBedrockUpstream(t *testing.T) (*httptest.Server, *atomic.Value) {
	t.Helper()
	var lastBody atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/model/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		auth := r.Header.Get("Authorization")
		if !sigV4Pattern.MatchString(auth) {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"message":"bad signature %q"}`, auth)
			return
		}
		if r.Header.Get("X-Amz-Date") == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		body, _ := io.ReadAll(r.Body)
		lastBody.Store(string(body))
		if strings.HasSuffix(r.URL.Path, "/invoke-with-response-stream") {
			w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
			w.WriteHeader(http.StatusOK)
			flusher := w.(http.Flusher)
			for _, payload := range []string{
				`{"type":"messageStart","role":"assistant"}`,
				`{"type":"contentBlockDelta","delta":{"text":"hello "}}`,
				`{"type":"contentBlockDelta","delta":{"text":"from bedrock"}}`,
				`{"type":"messageDelta","delta":{"stopReason":"end_turn"},"usage":{"outputTokens":50,"totalTokens":150}}`,
				`{"type":"messageStop"}`,
				`{"type":"metadata","usage":{"inputTokens":100,"outputTokens":50,"totalTokens":150}}`,
			} {
				_, _ = w.Write(aws.EncodeEventStreamMessage("chunk", []byte(payload)))
				flusher.Flush()
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"output":{"message":{"role":"assistant","content":[{"text":"hello from bedrock"}]}},"stopReason":"end_turn","usage":{"inputTokens":100,"outputTokens":50,"totalTokens":150}}`)
	}))
	t.Cleanup(srv.Close)
	return srv, &lastBody
}

// TestP3OpenAIInToClaudeChannel drives an OpenAI-format request into a
// Claude channel: request DTO conversion, x-api-key + anthropic-version
// auth headers, response conversion (non-stream + stream) and usage-based
// billing.
func TestP3OpenAIInToClaudeChannel(t *testing.T) {
	upstream, lastBody := newClaudeUpstream(t)
	env := setupRelayApp(t)
	createChannelID(t, env, map[string]any{
		"name": "claude-main", "type": 14, "key": claudeUpstreamKey,
		"base_url": upstream.URL, "models": "claude-3-5-sonnet",
	})

	chatBody := map[string]any{
		"model": "claude-3-5-sonnet",
		"messages": []map[string]any{
			{"role": "system", "content": "be brief"},
			{"role": "user", "content": "hi"},
		},
	}
	res := doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, chatBody)
	if res.Status != 200 {
		t.Fatalf("claude relay failed: %d %s", res.Status, res.Body)
	}
	if !strings.Contains(res.Body, "hello from claude") || !strings.Contains(res.Body, `"prompt_tokens":100`) {
		t.Fatalf("converted response wrong: %s", res.Body)
	}
	// the upstream received a proper Claude request
	sent := lastBody.Load().(string)
	if !strings.Contains(sent, `"system":"be brief"`) || !strings.Contains(sent, `"role":"user"`) {
		t.Fatalf("claude request conversion wrong: %s", sent)
	}
	expected := relay.ComputeQuota("claude-3-5-sonnet", "default", 100, 50)
	token, _ := model.GetTokenByID(env.tokenID)
	if token.RemainQuota != common.DefaultTokenQuota-expected {
		t.Fatalf("claude billing = %d, want %d", token.RemainQuota, common.DefaultTokenQuota-expected)
	}

	// stream: claude SSE -> openai SSE
	streamBody := map[string]any{
		"model":    "claude-3-5-sonnet",
		"stream":   true,
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	}
	res = doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, streamBody)
	if res.Status != 200 {
		t.Fatalf("claude stream relay failed: %d %s", res.Status, res.Body)
	}
	for _, want := range []string{`"role":"assistant"`, `"hello "`, `"from claude"`, `"prompt_tokens":100`, `"completion_tokens":50`, "[DONE]"} {
		if !strings.Contains(res.Body, want) {
			t.Fatalf("stream conversion missing %s: %s", want, res.Body)
		}
	}
}

// TestP3ClaudeInboundToOpenAIChannel is the ClaudeHelper direction of the
// P3.2 matrix: /v1/messages (Claude format) served by an OpenAI channel.
func TestP3ClaudeInboundToOpenAIChannel(t *testing.T) {
	upstream := newMockUpstream(t)
	env := setupRelayApp(t)
	createChannelID(t, env, map[string]any{
		"name": "openai-main", "key": upstreamKey, "base_url": upstream.URL,
		"models": "gpt-4o-mini",
	})

	// Claude SDK clients authenticate with x-api-key.
	claudeBody := map[string]any{
		"model":      "gpt-4o-mini",
		"max_tokens": 100,
		"system":     "You are helpful",
		"messages":   []map[string]string{{"role": "user", "content": "hi"}},
	}
	req, _ := json.Marshal(claudeBody)
	httpReq, _ := http.NewRequest("POST", env.base+"/v1/messages", bytes.NewReader(req))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", env.apiKey)
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatalf("claude-in request failed: %v", err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("claude-in bridge failed: %d %s", resp.StatusCode, raw)
	}
	body := string(raw)
	if !strings.Contains(body, `"type":"message"`) || !strings.Contains(body, "hello world") ||
		!strings.Contains(body, `"input_tokens":100`) || !strings.Contains(body, `"output_tokens":50`) {
		t.Fatalf("claude-in response wrong: %s", body)
	}
	expected := relay.ComputeQuota("gpt-4o-mini", "default", 100, 50)
	token, _ := model.GetTokenByID(env.tokenID)
	if token.RemainQuota != common.DefaultTokenQuota-expected {
		t.Fatalf("claude-in billing = %d, want %d", token.RemainQuota, common.DefaultTokenQuota-expected)
	}

	// stream: openai SSE -> claude SSE event sequence
	streamPayload := map[string]any{
		"model":      "gpt-4o-mini",
		"max_tokens": 100,
		"stream":     true,
		"messages":   []map[string]string{{"role": "user", "content": "hi"}},
	}
	res := doRequest(env.base, "POST", "/v1/messages", env.apiKey, streamPayload)
	if res.Status != 200 {
		t.Fatalf("claude-in stream failed: %d %s", res.Status, res.Body)
	}
	for _, want := range []string{
		"event: message_start", "event: content_block_start",
		"event: content_block_delta", `"text":"hello "`, `"text":"world"`,
		"event: message_delta", `"stop_reason":"end_turn"`, `"input_tokens":100`,
		"event: message_stop",
	} {
		if !strings.Contains(res.Body, want) {
			t.Fatalf("claude stream missing %s: %s", want, res.Body)
		}
	}
	token, _ = model.GetTokenByID(env.tokenID)
	if token.RemainQuota != common.DefaultTokenQuota-2*expected {
		t.Fatalf("claude-in stream billing = %d, want %d", token.RemainQuota, common.DefaultTokenQuota-2*expected)
	}

	// unknown model: the claude error envelope shape
	res = doRequest(env.base, "POST", "/v1/messages", env.apiKey,
		map[string]any{"model": "no-such-model", "max_tokens": 10,
			"messages": []map[string]string{{"role": "user", "content": "hi"}}})
	if res.Status != 503 || !strings.Contains(res.Body, `"type":"error"`) {
		t.Fatalf("claude-in 503 must use the claude envelope: %d %s", res.Status, res.Body)
	}

	// max_tokens bound: claude protocol field, same billing gate
	res = doRequest(env.base, "POST", "/v1/messages", env.apiKey,
		map[string]any{"model": "gpt-4o-mini", "max_tokens": common.MaxChatMaxTokens + 1,
			"messages": []map[string]string{{"role": "user", "content": "hi"}}})
	if res.Status != 400 {
		t.Fatalf("claude max_tokens bound must 400: %d %s", res.Status, res.Body)
	}
}

// TestP3ClaudeInboundPassthrough: claude-in on a native claude channel
// keeps the byte-level passthrough (no conversion round trip).
func TestP3ClaudeInboundPassthrough(t *testing.T) {
	upstream, lastBody := newClaudeUpstream(t)
	env := setupRelayApp(t)
	createChannelID(t, env, map[string]any{
		"name": "claude-native", "type": 14, "key": claudeUpstreamKey,
		"base_url": upstream.URL, "models": "claude-3-5-sonnet",
	})

	res := doRequest(env.base, "POST", "/v1/messages", env.apiKey, map[string]any{
		"model":      "claude-3-5-sonnet",
		"max_tokens": 100,
		"system":     "stay terse",
		"messages":   []map[string]string{{"role": "user", "content": "hi"}},
	})
	if res.Status != 200 || !strings.Contains(res.Body, "hello from claude") {
		t.Fatalf("claude passthrough failed: %d %s", res.Status, res.Body)
	}
	sent := lastBody.Load().(string)
	if !strings.Contains(sent, `"system":"stay terse"`) {
		t.Fatalf("passthrough must forward the body verbatim: %s", sent)
	}
	expected := relay.ComputeQuota("claude-3-5-sonnet", "default", 100, 50)
	token, _ := model.GetTokenByID(env.tokenID)
	if token.RemainQuota != common.DefaultTokenQuota-expected {
		t.Fatalf("passthrough billing = %d, want %d", token.RemainQuota, common.DefaultTokenQuota-expected)
	}

	// stream passthrough: the claude SSE frames flow through byte-faithfully
	res = doRequest(env.base, "POST", "/v1/messages", env.apiKey, map[string]any{
		"model":      "claude-3-5-sonnet",
		"max_tokens": 100,
		"stream":     true,
		"messages":   []map[string]string{{"role": "user", "content": "hi"}},
	})
	if res.Status != 200 {
		t.Fatalf("claude stream passthrough failed: %d %s", res.Status, res.Body)
	}
	for _, want := range []string{
		"event: message_start", "event: content_block_delta", `"text":"from claude"`,
		"event: message_delta", "event: message_stop",
	} {
		if !strings.Contains(res.Body, want) {
			t.Fatalf("claude stream passthrough missing %s: %s", want, res.Body)
		}
	}
	token, _ = model.GetTokenByID(env.tokenID)
	if token.RemainQuota != common.DefaultTokenQuota-2*expected {
		t.Fatalf("stream passthrough billing = %d, want %d", token.RemainQuota, common.DefaultTokenQuota-2*expected)
	}
}

// TestP3OpenAIInToGeminiChannel: openai-in to a Gemini channel (the
// "different path style" provider).
func TestP3OpenAIInToGeminiChannel(t *testing.T) {
	upstream := newGeminiUpstream(t)
	env := setupRelayApp(t)
	createChannelID(t, env, map[string]any{
		"name": "gemini-main", "type": 25, "key": geminiUpstreamKey,
		"base_url": upstream.URL, "models": "gemini-1.5-flash",
	})

	res := doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, map[string]any{
		"model": "gemini-1.5-flash",
		"messages": []map[string]any{
			{"role": "system", "content": "be brief"},
			{"role": "user", "content": "hi"},
		},
	})
	if res.Status != 200 {
		t.Fatalf("gemini relay failed: %d %s", res.Status, res.Body)
	}
	if !strings.Contains(res.Body, "hello from gemini") || !strings.Contains(res.Body, `"prompt_tokens":100`) {
		t.Fatalf("gemini conversion wrong: %s", res.Body)
	}
	expected := relay.ComputeQuota("gemini-1.5-flash", "default", 100, 50)
	token, _ := model.GetTokenByID(env.tokenID)
	if token.RemainQuota != common.DefaultTokenQuota-expected {
		t.Fatalf("gemini billing = %d, want %d", token.RemainQuota, common.DefaultTokenQuota-expected)
	}

	// stream: gemini alt=sse -> openai SSE
	res = doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, map[string]any{
		"model": "gemini-1.5-flash", "stream": true,
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	})
	if res.Status != 200 {
		t.Fatalf("gemini stream failed: %d %s", res.Status, res.Body)
	}
	for _, want := range []string{`"hello "`, `"from gemini"`, `"prompt_tokens":100`, "[DONE]"} {
		if !strings.Contains(res.Body, want) {
			t.Fatalf("gemini stream missing %s: %s", want, res.Body)
		}
	}
}

// TestP3OpenAIInToAWSChannel: openai-in to a Bedrock channel (SigV4 + the
// binary event stream).
func TestP3OpenAIInToAWSChannel(t *testing.T) {
	upstream, lastBody := newBedrockUpstream(t)
	env := setupRelayApp(t)
	createChannelID(t, env, map[string]any{
		"name": "bedrock-main", "type": 15,
		"key":      bedrockAccessKey + "|" + bedrockSecretKey + "|" + bedrockRegion,
		"base_url": upstream.URL, "models": "anthropic.claude-3-5-sonnet",
	})

	res := doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, map[string]any{
		"model": "anthropic.claude-3-5-sonnet",
		"messages": []map[string]any{
			{"role": "system", "content": "be brief"},
			{"role": "user", "content": "hi"},
		},
	})
	if res.Status != 200 {
		t.Fatalf("bedrock relay failed: %d %s", res.Status, res.Body)
	}
	if !strings.Contains(res.Body, "hello from bedrock") || !strings.Contains(res.Body, `"prompt_tokens":100`) {
		t.Fatalf("bedrock conversion wrong: %s", res.Body)
	}
	sent := lastBody.Load().(string)
	if !strings.Contains(sent, `"system":[{"text":"be brief"}]`) || !strings.Contains(sent, `"role":"user"`) {
		t.Fatalf("bedrock request conversion wrong: %s", sent)
	}
	expected := relay.ComputeQuota("anthropic.claude-3-5-sonnet", "default", 100, 50)
	token, _ := model.GetTokenByID(env.tokenID)
	if token.RemainQuota != common.DefaultTokenQuota-expected {
		t.Fatalf("bedrock billing = %d, want %d", token.RemainQuota, common.DefaultTokenQuota-expected)
	}

	// stream: the AWS event stream converts into openai SSE
	res = doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, map[string]any{
		"model": "anthropic.claude-3-5-sonnet", "stream": true,
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	})
	if res.Status != 200 {
		t.Fatalf("bedrock stream failed: %d %s", res.Status, res.Body)
	}
	for _, want := range []string{`"hello "`, `"from bedrock"`, `"prompt_tokens":100`, "[DONE]"} {
		if !strings.Contains(res.Body, want) {
			t.Fatalf("bedrock stream missing %s: %s", want, res.Body)
		}
	}
}

// TestP3ChannelTypesAndProbe: channel creation validates the new types and
// the probe converts its probe per protocol.
func TestP3ChannelTypesAndProbe(t *testing.T) {
	claudeUpstream, _ := newClaudeUpstream(t)
	env := setupRelayApp(t)

	// an invalid type is rejected with the supported list
	res := doRequest(env.base, "POST", "/api/channel/", env.session, map[string]any{
		"name": "bad-type", "type": 99, "key": "k", "base_url": claudeUpstream.URL, "models": "m",
	})
	if res.Status != 400 || !strings.Contains(res.Body, "14") {
		t.Fatalf("invalid channel type should 400 listing supported types: %d %s", res.Status, res.Body)
	}

	chID := createChannelID(t, env, map[string]any{
		"name": "claude-probe", "type": 14, "key": claudeUpstreamKey,
		"base_url": claudeUpstream.URL, "models": "claude-3-5-sonnet",
	})
	// the probe speaks the channel protocol (converted IR probe)
	res = doRequest(env.base, "POST", "/api/channel/test/"+fmt.Sprint(chID), env.session, nil)
	if res.Status != 200 || !strings.Contains(res.Body, `"ok":true`) {
		t.Fatalf("claude channel probe failed: %d %s", res.Status, res.Body)
	}

	// unsupported mode: embeddings on a claude channel gives a clear 400
	res = doRequest(env.base, "POST", "/v1/embeddings", env.apiKey, map[string]any{
		"model": "claude-3-5-sonnet", "input": "hello",
	})
	if res.Status != 400 || !strings.Contains(res.Body, "unsupported_mode") {
		t.Fatalf("embeddings on claude channel should be a clean 400: %d %s", res.Status, res.Body)
	}
}

// ---------------------------------------------------------------------------
// P3 passthrough endpoints: embeddings / rerank / audio
// ---------------------------------------------------------------------------

func newOpenAIExtraUpstream(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+upstreamKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/v1/embeddings":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"object":"list","data":[{"object":"embedding","embedding":[0.1,0.2],"index":0}],"model":"text-embedding-3-small","usage":{"prompt_tokens":123,"total_tokens":123}}`)
		case "/v1/rerank":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"results":[{"index":1,"relevance_score":0.98},{"index":0,"relevance_score":0.4}],"usage":{"input_tokens":77,"output_tokens":0}}`)
		case "/v1/audio/speech":
			w.Header().Set("Content-Type", "audio/mpeg")
			fmt.Fprint(w, "fake-mp3-bytes")
		case "/v1/audio/transcriptions", "/v1/audio/translations":
			// verify the multipart body arrived intact
			form, err := parseTestMultipart(r)
			if err != nil || form.Value("model") == "" || form.File("file") == nil {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprintf(w, `{"error":{"message":"multipart incomplete: %v model=%q"}}`, err, form.Value("model"))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"text":"transcribed words","duration":6.0}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func parseTestMultipart(r *http.Request) (*common.MultipartForm, error) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 32<<20))
	return common.ParseMultipartForm(r.Header.Get("Content-Type"), body)
}

func TestP3EmbeddingsAndRerank(t *testing.T) {
	upstream := newOpenAIExtraUpstream(t)
	env := setupRelayApp(t)
	createChannelID(t, env, map[string]any{
		"name": "openai-extra", "key": upstreamKey, "base_url": upstream.URL,
		"models": "text-embedding-3-small,rerank-model",
	})

	res := doRequest(env.base, "POST", "/v1/embeddings", env.apiKey, map[string]any{
		"model": "text-embedding-3-small", "input": []string{"hello", "world"},
	})
	if res.Status != 200 || !strings.Contains(res.Body, `"prompt_tokens":123`) {
		t.Fatalf("embeddings relay failed: %d %s", res.Status, res.Body)
	}
	embeddingCost := relay.ComputeQuota("text-embedding-3-small", "default", 123, 0)
	token, _ := model.GetTokenByID(env.tokenID)
	if token.RemainQuota != common.DefaultTokenQuota-embeddingCost {
		t.Fatalf("embeddings billing = %d, want %d", token.RemainQuota, common.DefaultTokenQuota-embeddingCost)
	}

	res = doRequest(env.base, "POST", "/v1/rerank", env.apiKey, map[string]any{
		"model": "rerank-model", "query": "hello",
		"documents": []string{"doc one", "doc two"},
	})
	if res.Status != 200 || !strings.Contains(res.Body, `"relevance_score":0.98`) {
		t.Fatalf("rerank relay failed: %d %s", res.Status, res.Body)
	}
	rerankCost := relay.ComputeQuota("rerank-model", "default", 77, 0)
	token, _ = model.GetTokenByID(env.tokenID)
	if token.RemainQuota != common.DefaultTokenQuota-embeddingCost-rerankCost {
		t.Fatalf("rerank billing = %d, want %d", token.RemainQuota, common.DefaultTokenQuota-embeddingCost-rerankCost)
	}

	// validation gates
	res = doRequest(env.base, "POST", "/v1/embeddings", env.apiKey, map[string]any{"model": "text-embedding-3-small"})
	if res.Status != 400 {
		t.Fatalf("embeddings without input must 400: %d", res.Status)
	}
	res = doRequest(env.base, "POST", "/v1/rerank", env.apiKey, map[string]any{"model": "rerank-model", "query": "q"})
	if res.Status != 400 {
		t.Fatalf("rerank without documents must 400: %d", res.Status)
	}
}

func buildMultipartBody(t *testing.T, fields map[string]string, fileField, fileName string, fileContent []byte) (io.Reader, string) {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := writer.WriteField(k, v); err != nil {
			t.Fatalf("write field: %v", err)
		}
	}
	part, err := writer.CreateFormFile(fileField, fileName)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(fileContent); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return bytes.NewReader(buf.Bytes()), writer.FormDataContentType()
}

func postRaw(base, path, apiKey, contentType string, body io.Reader) apiResult {
	req, _ := http.NewRequest("POST", base+path, body)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return apiResult{Status: -1, Body: err.Error()}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return apiResult{Status: resp.StatusCode, Body: string(raw)}
}

func TestP3AudioTTSAndSTT(t *testing.T) {
	upstream := newOpenAIExtraUpstream(t)
	env := setupRelayApp(t)
	createChannelID(t, env, map[string]any{
		"name": "openai-audio", "key": upstreamKey, "base_url": upstream.URL,
		"models": "tts-1,whisper-1",
	})
	setOption(t, env, "model_price", `{"tts-1":0.01,"whisper-1":0.006}`)

	// TTS: audio bytes passthrough + per-call billing
	res := doRequest(env.base, "POST", "/v1/audio/speech", env.apiKey, map[string]any{
		"model": "tts-1", "input": "say something nice", "voice": "alloy",
	})
	if res.Status != 200 || res.Body != "fake-mp3-bytes" {
		t.Fatalf("tts relay failed: %d %q", res.Status, res.Body)
	}
	token, _ := model.GetTokenByID(env.tokenID)
	if token.RemainQuota != common.DefaultTokenQuota-5000 {
		t.Fatalf("tts per-call billing = %d, want %d", token.RemainQuota, common.DefaultTokenQuota-5000)
	}
	// input length bound
	res = doRequest(env.base, "POST", "/v1/audio/speech", env.apiKey, map[string]any{
		"model": "tts-1", "input": strings.Repeat("x", 5000),
	})
	if res.Status != 400 {
		t.Fatalf("tts input bound must 400: %d", res.Status)
	}

	// STT: multipart passthrough + per-call billing (the duration-based
	// usage path applies when no price is configured)
	body, contentType := buildMultipartBody(t, map[string]string{
		"model":           "whisper-1",
		"response_format": "verbose_json",
	}, "file", "audio.mp3", []byte("fake-audio-bytes"))
	res = postRaw(env.base, "/v1/audio/transcriptions", env.apiKey, contentType, body)
	if res.Status != 200 || !strings.Contains(res.Body, "transcribed words") {
		t.Fatalf("stt relay failed: %d %s", res.Status, res.Body)
	}
	token, _ = model.GetTokenByID(env.tokenID)
	if token.RemainQuota != common.DefaultTokenQuota-5000-3000 {
		t.Fatalf("stt per-call billing = %d, want %d", token.RemainQuota, common.DefaultTokenQuota-8000)
	}

	// STT missing file: rejected before billing
	body, contentType = buildMultipartBody(t, map[string]string{"model": "whisper-1"}, "notfile", "x", []byte("y"))
	res = postRaw(env.base, "/v1/audio/transcriptions", env.apiKey, contentType, body)
	if res.Status != 400 {
		t.Fatalf("stt without file must 400: %d %s", res.Status, res.Body)
	}
}

// ---------------------------------------------------------------------------
// P3.1 image relay
// ---------------------------------------------------------------------------

// newImageUpstream serves /v1/images/generations and /v1/images/edits with
// a configurable delivered count.
func newImageUpstream(t *testing.T, delivered int) (*httptest.Server, *atomic.Value) {
	t.Helper()
	var requests atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+upstreamKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/v1/images/generations":
			body, _ := io.ReadAll(r.Body)
			requests.Store(string(body))
			var req struct {
				N      *int  `json:"n"`
				Stream *bool `json:"stream"`
			}
			_ = json.Unmarshal(body, &req)
			if req.Stream != nil && *req.Stream {
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				flusher := w.(http.Flusher)
				for i := 0; i < delivered; i++ {
					fmt.Fprintf(w, "event: image_generation.partial_image\ndata: {\"type\":\"image_generation.partial_image\",\"output_index\":%d,\"b64_json\":\"cGFydGlhbA==\"}\n\n", i)
					fmt.Fprintf(w, "event: image_generation.completed\ndata: {\"type\":\"image_generation.completed\",\"output_index\":%d,\"b64_json\":\"QUJD\"}\n\n", i)
				}
				fmt.Fprint(w, "data: {\"usage\":{\"input_tokens\":10,\"output_tokens\":40,\"total_tokens\":50}}\n\n")
				fmt.Fprint(w, "data: [DONE]\n\n")
				flusher.Flush()
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"created":1700000000,"data":[`)
			for i := 0; i < delivered; i++ {
				if i > 0 {
					fmt.Fprint(w, ",")
				}
				fmt.Fprintf(w, `{"url":"https://cdn.example/img-%d.png"}`, i)
			}
			fmt.Fprint(w, `],"usage":{"input_tokens":10,"output_tokens":40,"total_tokens":50}}`)
		case "/v1/images/edits":
			// the multipart body must arrive intact
			body, _ := io.ReadAll(io.LimitReader(r.Body, 32<<20))
			form, err := common.ParseMultipartForm(r.Header.Get("Content-Type"), body)
			if err != nil || form.Value("model") == "" || form.File("image") == nil {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprintf(w, `{"error":{"message":"edits multipart incomplete"}}`)
				return
			}
			n := 1
			if raw := form.Value("n"); raw != "" {
				fmt.Sscanf(raw, "%d", &n)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"created":1700000000,"data":[`)
			for i := 0; i < n; i++ {
				if i > 0 {
					fmt.Fprint(w, ",")
				}
				fmt.Fprintf(w, `{"b64_json":"RklY"}`)
			}
			fmt.Fprint(w, `]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &requests
}

func TestP3ImageGenerations(t *testing.T) {
	upstream, requests := newImageUpstream(t, 2)
	env := setupRelayApp(t)
	createChannelID(t, env, map[string]any{
		"name": "openai-image", "key": upstreamKey, "base_url": upstream.URL,
		"models": "gpt-image-1",
	})
	setOption(t, env, "model_price", `{"gpt-image-1":0.04}`)
	remain := func() int64 {
		token, _ := model.GetTokenByID(env.tokenID)
		return token.RemainQuota
	}

	// per-call billing: 2 images x $0.04
	res := doRequest(env.base, "POST", "/v1/images/generations", env.apiKey, map[string]any{
		"model": "gpt-image-1", "prompt": "a cat", "n": 2, "size": "1024x1024",
	})
	if res.Status != 200 || !strings.Contains(res.Body, "img-1.png") {
		t.Fatalf("image generations failed: %d %s", res.Status, res.Body)
	}
	sent := requests.Load().(string)
	if !strings.Contains(sent, `"n":2`) {
		t.Fatalf("request body must be forwarded verbatim: %s", sent)
	}
	if got := remain(); got != common.DefaultTokenQuota-int64(0.04*2*500000) {
		t.Fatalf("image billing = %d, want %d", got, common.DefaultTokenQuota-int64(0.04*2*500000))
	}

	// stream: partial events forward free, completed events bill
	res = doRequest(env.base, "POST", "/v1/images/generations", env.apiKey, map[string]any{
		"model": "gpt-image-1", "prompt": "a cat", "n": 2, "stream": true,
	})
	if res.Status != 200 || !strings.Contains(res.Body, "event: image_generation.completed") ||
		!strings.Contains(res.Body, "image_generation.partial_image") {
		t.Fatalf("image stream failed: %d %s", res.Status, res.Body)
	}
	if got := remain(); got != common.DefaultTokenQuota-int64(0.04*2*500000*2) {
		t.Fatalf("image stream billing = %d, want %d", got, common.DefaultTokenQuota-int64(0.04*2*500000*2))
	}

	// validation: n bound and the full-width size hint
	res = doRequest(env.base, "POST", "/v1/images/generations", env.apiKey, map[string]any{
		"model": "gpt-image-1", "prompt": "x", "n": 200,
	})
	if res.Status != 400 {
		t.Fatalf("image n bound must 400: %d", res.Status)
	}
	res = doRequest(env.base, "POST", "/v1/images/generations", env.apiKey, map[string]any{
		"model": "gpt-image-1", "prompt": "x", "size": "1024×1024",
	})
	if res.Status != 400 || !strings.Contains(res.Body, "full-width") {
		t.Fatalf("full-width size must 400 with a hint: %d %s", res.Status, res.Body)
	}
}

func TestP3ImageOverAndUnderDelivery(t *testing.T) {
	// upstream delivers MORE images than requested: the actual count bills
	// (P3.1 invariant: upstream over-delivery must not be free)
	over, _ := newImageUpstream(t, 3)
	env := setupRelayApp(t)
	createChannelID(t, env, map[string]any{
		"name": "over", "key": upstreamKey, "base_url": over.URL, "models": "gpt-image-1",
	})
	setOption(t, env, "model_price", `{"gpt-image-1":0.04}`)
	res := doRequest(env.base, "POST", "/v1/images/generations", env.apiKey, map[string]any{
		"model": "gpt-image-1", "prompt": "a cat", "n": 2,
	})
	if res.Status != 200 {
		t.Fatalf("over-delivery relay failed: %d %s", res.Status, res.Body)
	}
	token, _ := model.GetTokenByID(env.tokenID)
	if token.RemainQuota != common.DefaultTokenQuota-int64(0.04*3*500000) {
		t.Fatalf("over-delivery billing = %d, want %d (3 images)", token.RemainQuota, common.DefaultTokenQuota-int64(0.04*3*500000))
	}
}

func TestP3ImageEditsMultipart(t *testing.T) {
	upstream, _ := newImageUpstream(t, 1)
	env := setupRelayApp(t)
	createChannelID(t, env, map[string]any{
		"name": "openai-image-edit", "key": upstreamKey, "base_url": upstream.URL,
		"models": "gpt-image-1",
	})
	setOption(t, env, "model_price", `{"gpt-image-1":0.04}`)

	body, contentType := buildMultipartBody(t, map[string]string{
		"model":  "gpt-image-1",
		"prompt": "make the cat wear a hat",
		"n":      "2",
	}, "image", "cat.png", []byte("fake-png-bytes"))
	res := postRaw(env.base, "/v1/images/edits", env.apiKey, contentType, body)
	if res.Status != 200 || !strings.Contains(res.Body, "b64_json") {
		t.Fatalf("image edits failed: %d %s", res.Status, res.Body)
	}
	token, _ := model.GetTokenByID(env.tokenID)
	if token.RemainQuota != common.DefaultTokenQuota-int64(0.04*2*500000) {
		t.Fatalf("image edits billing (n from form) = %d, want %d", token.RemainQuota, common.DefaultTokenQuota-int64(0.04*2*500000))
	}

	// the multipart n bound shares the JSON path's gate
	body, contentType = buildMultipartBody(t, map[string]string{
		"model": "gpt-image-1", "prompt": "x", "n": "500",
	}, "image", "cat.png", []byte("fake-png-bytes"))
	res = postRaw(env.base, "/v1/images/edits", env.apiKey, contentType, body)
	if res.Status != 400 {
		t.Fatalf("multipart n bound must 400: %d %s", res.Status, res.Body)
	}
}

func TestP3ImageJSONasSSE(t *testing.T) {
	// the client asks for a stream; the upstream answers plain JSON: the
	// gateway wraps the JSON into completed events (its protocol promise
	// to the client outranks the upstream's capabilities, P3.1).
	var sawStreamFlag atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/generations" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Stream *bool `json:"stream"`
		}
		_ = json.Unmarshal(body, &req)
		if req.Stream != nil && *req.Stream {
			sawStreamFlag.Store(true)
		}
		// answer JSON regardless of the stream flag
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"created":1700000000,"data":[{"url":"https://cdn.example/a.png"},{"url":"https://cdn.example/b.png"}],"usage":{"input_tokens":10,"output_tokens":40}}`)
	}))
	t.Cleanup(upstream.Close)

	env := setupRelayApp(t)
	createChannelID(t, env, map[string]any{
		"name": "json-only", "key": upstreamKey, "base_url": upstream.URL, "models": "gpt-image-1",
	})
	setOption(t, env, "model_price", `{"gpt-image-1":0.04}`)
	res := doRequest(env.base, "POST", "/v1/images/generations", env.apiKey, map[string]any{
		"model": "gpt-image-1", "prompt": "a cat", "n": 2, "stream": true,
	})
	if res.Status != 200 {
		t.Fatalf("json-as-sse failed: %d %s", res.Status, res.Body)
	}
	if !strings.Contains(res.Body, "event: image_generation.completed") ||
		!strings.Contains(res.Body, "a.png") || !strings.Contains(res.Body, "[DONE]") {
		t.Fatalf("json response must be wrapped into SSE: %s", res.Body)
	}
	if !sawStreamFlag.Load() {
		t.Fatal("the stream flag must reach the upstream")
	}
	token, _ := model.GetTokenByID(env.tokenID)
	if token.RemainQuota != common.DefaultTokenQuota-int64(0.04*2*500000) {
		t.Fatalf("json-as-sse billing = %d", token.RemainQuota)
	}
}

// TestP3CrossProtocolFailover: one request, two channels speaking
// different protocols — the first (claude) 500s, the failover converts to
// the second (openai) protocol mid-request.
func TestP3CrossProtocolFailover(t *testing.T) {
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"type":"error","error":{"type":"overloaded_error","message":"overloaded"}}`)
	}))
	t.Cleanup(broken.Close)
	healthy := newMockUpstream(t)

	env := setupRelayApp(t)
	claudeCh := createChannelID(t, env, map[string]any{
		"name": "claude-500", "type": 14, "key": claudeUpstreamKey,
		"base_url": broken.URL, "models": "gpt-4o-mini", "priority": 20,
	})
	openaiCh := createChannelID(t, env, map[string]any{
		"name": "openai-ok", "key": upstreamKey, "base_url": healthy.URL,
		"models": "gpt-4o-mini", "priority": 10,
	})
	res := doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, map[string]any{
		"model":    "gpt-4o-mini",
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	})
	if res.Status != 200 || !strings.Contains(res.Body, "hello world") {
		t.Fatalf("cross-protocol failover failed: %d %s", res.Status, res.Body)
	}
	logs, total, err := model.ListLogs(env.rootID, 1, 10, "gpt-4o-mini")
	if err != nil || total != 1 {
		t.Fatalf("failover log missing: %v", err)
	}
	if want := fmt.Sprintf("%d->%d", claudeCh, openaiCh); logs[0].UsedChannels != want {
		t.Fatalf("retry trace = %q, want %q", logs[0].UsedChannels, want)
	}
}

// ---------------------------------------------------------------------------
// P3.3 task lifecycle
// ---------------------------------------------------------------------------

type videoTaskMock struct {
	// status sequence returned by successive polls
	sequence []string
	progress []int
	videoURL string
	calls    atomic.Int32
}

func (m *videoTaskMock) handler(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+upstreamKey {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	switch {
	case r.URL.Path == "/v1/videos" && r.Method == http.MethodPost:
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Model   string `json:"model"`
			Prompt  string `json:"prompt"`
			Seconds int    `json:"seconds"`
			N       int    `json:"n"`
		}
		_ = json.Unmarshal(body, &req)
		if req.Model == "" || req.Prompt == "" || req.Seconds == 0 || req.N == 0 {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":{"message":"video submit fields missing"}}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"video_42","status":"queued","progress":0}`)
	case strings.HasPrefix(r.URL.Path, "/v1/videos/") && r.Method == http.MethodGet:
		n := int(m.calls.Add(1)) - 1
		idx := n
		if idx >= len(m.sequence) {
			idx = len(m.sequence) - 1
		}
		status := m.sequence[idx]
		progress := 0
		if idx < len(m.progress) {
			progress = m.progress[idx]
		}
		w.Header().Set("Content-Type", "application/json")
		if status == "completed" {
			fmt.Fprintf(w, `{"id":"video_42","status":"completed","progress":100,"video":{"url":%q}}`, m.videoURL)
			return
		}
		if status == "failed" {
			fmt.Fprint(w, `{"id":"video_42","status":"failed","error":{"code":"content_policy_violation","message":"prompt rejected"}}`)
			return
		}
		fmt.Fprintf(w, `{"id":"video_42","status":"in_progress","progress":%d}`, progress)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestP3TaskLifecycle(t *testing.T) {
	mock := &videoTaskMock{
		sequence: []string{"in_progress", "in_progress", "completed"},
		progress: []int{10, 60, 100},
		videoURL: "https://cdn.example/video_42.mp4",
	}
	upstream := httptest.NewServer(http.HandlerFunc(mock.handler))
	t.Cleanup(upstream.Close)

	env := setupRelayApp(t)
	createChannelID(t, env, map[string]any{
		"name": "openai-video", "key": upstreamKey, "base_url": upstream.URL,
		"models": "sora-2",
	})
	setOption(t, env, "model_price", `{"sora-2":0.01}`)
	remain := func() int64 {
		token, _ := model.GetTokenByID(env.tokenID)
		return token.RemainQuota
	}
	// the whole lifecycle bills 8s x 2 x $0.01/s = $0.16 = 80000 quota
	taskCost := int64(0.01 * 8 * 2 * 500000)

	// submit: 8 seconds x 2 videos x $0.01/s pre-charge
	res := doRequest(env.base, "POST", "/v1/video/generations", env.apiKey, map[string]any{
		"model": "sora-2", "prompt": "a cat surfing", "seconds": 8, "size": "720x1280", "n": 2,
	})
	if res.Status != 200 {
		t.Fatalf("task submit failed: %d %s", res.Status, res.Body)
	}
	var submit struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(res.Body), &submit); err != nil || submit.ID == "" {
		t.Fatalf("bad submit response: %s", res.Body)
	}
	if got := remain(); got != common.DefaultTokenQuota-taskCost {
		t.Fatalf("task pre-charge = %d, want %d", got, common.DefaultTokenQuota-taskCost)
	}

	// poll 1: in progress, progress field flows through, money stays held
	res = doRequest(env.base, "GET", "/v1/video/generations/"+submit.ID, env.apiKey, nil)
	if res.Status != 200 || !strings.Contains(res.Body, `"status":"in_progress"`) || !strings.Contains(res.Body, `"progress":10`) {
		t.Fatalf("task poll 1 wrong: %d %s", res.Status, res.Body)
	}
	if got := remain(); got != common.DefaultTokenQuota-taskCost {
		t.Fatalf("in-progress task must keep holding the pre-charge: %d", got)
	}

	// poll 2..3: completed; the pre-charge settles at the same price
	res = doRequest(env.base, "GET", "/v1/video/generations/"+submit.ID, env.apiKey, nil)
	res = doRequest(env.base, "GET", "/v1/video/generations/"+submit.ID, env.apiKey, nil)
	if res.Status != 200 || !strings.Contains(res.Body, `"status":"succeeded"`) ||
		!strings.Contains(res.Body, "video_42.mp4") {
		t.Fatalf("task completion wrong: %d %s", res.Status, res.Body)
	}
	if got := remain(); got != common.DefaultTokenQuota-taskCost {
		t.Fatalf("completed task billing = %d, want %d", got, common.DefaultTokenQuota-taskCost)
	}
	// the consumption log carries the task model
	logs, total, err := model.ListLogs(env.rootID, 1, 10, "sora-2")
	if err != nil || total != 1 {
		t.Fatalf("task log missing: total=%d err=%v", total, err)
	}
	if logs[0].Quota != taskCost {
		t.Fatalf("task log quota = %d, want %d", logs[0].Quota, taskCost)
	}
}

func TestP3TaskFailureRefundsAndReaper(t *testing.T) {
	mock := &videoTaskMock{
		sequence: []string{"in_progress", "failed"},
		progress: []int{20, 0},
	}
	upstream := httptest.NewServer(http.HandlerFunc(mock.handler))
	t.Cleanup(upstream.Close)

	env := setupRelayApp(t)
	createChannelID(t, env, map[string]any{
		"name": "openai-video", "key": upstreamKey, "base_url": upstream.URL,
		"models": "sora-2",
	})
	setOption(t, env, "model_price", `{"sora-2":0.01}`)
	remain := func() int64 {
		token, _ := model.GetTokenByID(env.tokenID)
		return token.RemainQuota
	}
	taskCost := int64(0.01 * 4 * 1 * 500000)

	// a failed task refunds the whole pre-charge
	res := doRequest(env.base, "POST", "/v1/video/generations", env.apiKey, map[string]any{
		"model": "sora-2", "prompt": "bad prompt", "seconds": 4, "n": 1,
	})
	if res.Status != 200 {
		t.Fatalf("submit failed: %d %s", res.Status, res.Body)
	}
	var submit struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal([]byte(res.Body), &submit)
	if got := remain(); got != common.DefaultTokenQuota-taskCost {
		t.Fatalf("pre-charge = %d, want %d", got, common.DefaultTokenQuota-taskCost)
	}
	// poll once (in progress), then again (failed)
	res = doRequest(env.base, "GET", "/v1/video/generations/"+submit.ID, env.apiKey, nil)
	res = doRequest(env.base, "GET", "/v1/video/generations/"+submit.ID, env.apiKey, nil)
	if !strings.Contains(res.Body, `"status":"failed"`) || !strings.Contains(res.Body, "prompt rejected") {
		t.Fatalf("failed task view wrong: %s", res.Body)
	}
	if got := remain(); got != common.DefaultTokenQuota {
		t.Fatalf("failed task must be fully refunded: %d, want %d", got, common.DefaultTokenQuota)
	}

	// the reaper finishes tasks whose owners never poll
	reapMock := &videoTaskMock{
		sequence: []string{"completed"},
		progress: []int{100},
		videoURL: "https://cdn.example/reaped.mp4",
	}
	reapUpstream := httptest.NewServer(http.HandlerFunc(reapMock.handler))
	t.Cleanup(reapUpstream.Close)
	createChannelID(t, env, map[string]any{
		"name": "video-2", "key": upstreamKey, "base_url": reapUpstream.URL, "models": "sora-2-pro",
	})
	setOption(t, env, "model_price", `{"sora-2":0.01,"sora-2-pro":0.01}`)
	res = doRequest(env.base, "POST", "/v1/video/generations", env.apiKey, map[string]any{
		"model": "sora-2-pro", "prompt": "reap me", "seconds": 4, "n": 1,
	})
	if res.Status != 200 {
		t.Fatalf("reaper submit failed: %d %s", res.Status, res.Body)
	}
	_ = json.Unmarshal([]byte(res.Body), &submit)
	if got := remain(); got != common.DefaultTokenQuota-taskCost {
		t.Fatalf("reaper pre-charge = %d, want %d", got, common.DefaultTokenQuota-taskCost)
	}
	service.PollPendingTasksOnce(t.Context())
	task, err := model.GetTaskByID(parseTaskID(t, submit.ID))
	if err != nil || task.Status != "succeeded" {
		t.Fatalf("reaper must complete the task: %+v %v", task, err)
	}
	if got := remain(); got != common.DefaultTokenQuota-taskCost {
		t.Fatalf("reaper settlement = %d, want %d", got, common.DefaultTokenQuota-taskCost)
	}

	// submit validation gates
	res = doRequest(env.base, "POST", "/v1/video/generations", env.apiKey, map[string]any{
		"model": "sora-2", "prompt": "x", "seconds": 120,
	})
	if res.Status != 400 {
		t.Fatalf("seconds bound must 400: %d %s", res.Status, res.Body)
	}
}

func parseTaskID(t *testing.T, raw string) int64 {
	t.Helper()
	var id int64
	if _, err := fmt.Sscanf(raw, "%d", &id); err != nil {
		t.Fatalf("bad task id %q", raw)
	}
	return id
}

// ---------------------------------------------------------------------------
// P3 realtime websocket
// ---------------------------------------------------------------------------

func TestP3RealtimeRelay(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/realtime" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Query().Get("model") != "gpt-4o-realtime" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+upstreamKey {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		conn, err := relay.WSAccept(w, r)
		if err != nil {
			return
		}
		defer conn.Close()
		// read one client frame (the gateway forwards it masked)
		conn.SetReadDeadline(5 * time.Second)
		frame, err := conn.ReadFrame()
		if err != nil {
			return
		}
		// answer with a conversation event and a terminal response.done
		// carrying the usage that settles the session
		_ = conn.WriteFrame(relay.WSOpText, append([]byte(`{"type":"response.output_text.delta","delta":"echo `), append(frame.Data, '"')...))
		_ = conn.WriteFrame(relay.WSOpText, []byte(`{"type":"response.done","response":{"usage":{"input_tokens":200,"output_tokens":100}}}`))
		time.Sleep(100 * time.Millisecond) // let the frames drain
	}))
	t.Cleanup(upstream.Close)

	env := setupRelayApp(t)
	createChannelID(t, env, map[string]any{
		"name": "openai-realtime", "key": upstreamKey, "base_url": upstream.URL,
		"models": "gpt-4o-realtime",
	})

	wsURL := "ws" + strings.TrimPrefix(env.base, "http") + "/v1/realtime?model=gpt-4o-realtime"
	client, err := relay.DialWebSocket(wsURL, http.Header{
		"Authorization": []string{"Bearer " + env.apiKey},
	})
	if err != nil {
		t.Fatalf("realtime dial failed: %v", err)
	}
	if err := client.WriteFrame(relay.WSOpText, []byte("hi")); err != nil {
		t.Fatalf("realtime write failed: %v", err)
	}
	client.SetReadDeadline(5 * time.Second)
	var sawEcho, sawUsage bool
	for !(sawEcho && sawUsage) {
		frame, err := client.ReadFrame()
		if err != nil {
			t.Fatalf("realtime read failed (echo=%v usage=%v): %v", sawEcho, sawUsage, err)
		}
		if !strings.Contains(string(frame.Data), "echo hi") {
			t.Fatalf("unexpected realtime frame: %s", frame.Data)
		}
		sawEcho = true
		frame, err = client.ReadFrame()
		if err != nil {
			t.Fatalf("realtime usage read failed: %v", err)
		}
		if !strings.Contains(string(frame.Data), `"input_tokens":200`) {
			t.Fatalf("unexpected usage frame: %s", frame.Data)
		}
		sawUsage = true
	}
	client.Close()

	// settle on the response.done usage: (200 prompt + 100 completion x
	// completion ratio) x model ratio
	expected := relay.ComputeQuota("gpt-4o-realtime", "default", 200, 100)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		token, _ := model.GetTokenByID(env.tokenID)
		if token.RemainQuota == common.DefaultTokenQuota-expected {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	token, _ := model.GetTokenByID(env.tokenID)
	t.Fatalf("realtime billing = %d, want %d", token.RemainQuota, common.DefaultTokenQuota-expected)
}

// TestP3RealtimeModelRequired: the query model is validated before the
// upgrade.
func TestP3RealtimeModelRequired(t *testing.T) {
	env := setupRelayApp(t)
	res := doRequest(env.base, "GET", "/v1/realtime", env.apiKey, nil)
	if res.Status != 400 || !strings.Contains(res.Body, "model_required") {
		t.Fatalf("realtime without model must 400: %d %s", res.Status, res.Body)
	}
	// no channel for the model: clean 502 before any upgrade
	res = doRequest(env.base, "GET", "/v1/realtime?model=nonexistent-realtime", env.apiKey, nil)
	if res.Status != 502 {
		t.Fatalf("realtime without channel must 502: %d %s", res.Status, res.Body)
	}
}

// TestP3RateLimitAppliesToNewEndpoints: the new relay family sits behind
// the same per-user limiter.
func TestP3RateLimitAppliesToNewEndpoints(t *testing.T) {
	upstream := newOpenAIExtraUpstream(t)
	env := setupRelayApp(t)
	createChannelID(t, env, map[string]any{
		"name": "limited", "key": upstreamKey, "base_url": upstream.URL,
		"models": "text-embedding-3-small",
	})
	middleware.ConfigureRateLimits(middleware.DefaultIPLimitPerMinute, 2)
	middleware.ResetRateLimits()
	t.Cleanup(func() {
		middleware.ConfigureRateLimits(middleware.DefaultIPLimitPerMinute, middleware.DefaultUserLimitPerMinute)
		middleware.ResetRateLimits()
	})
	for i := 0; i < 2; i++ {
		res := doRequest(env.base, "POST", "/v1/embeddings", env.apiKey,
			map[string]any{"model": "text-embedding-3-small", "input": "x"})
		if res.Status != 200 {
			t.Fatalf("embedding %d under the limit failed: %d", i, res.Status)
		}
	}
	res := doRequest(env.base, "POST", "/v1/embeddings", env.apiKey,
		map[string]any{"model": "text-embedding-3-small", "input": "x"})
	if res.Status != 429 {
		t.Fatalf("third embedding must hit the limiter: %d", res.Status)
	}
}

// TestP3DBMigration smoke-checks the Task table exists through the normal
// InitDB path (covered by every other test, kept explicit for clarity).
func TestP3DBMigration(t *testing.T) {
	if err := model.InitDB(filepath.Join(t.TempDir(), "p3.db")); err != nil {
		t.Fatalf("InitDB with task table failed: %v", err)
	}
	t.Cleanup(func() { _ = model.CloseDB() })
	if err := model.InsertTask(&model.Task{TaskID: "t", Platform: "sora", Action: "video", Status: "submitted"}); err != nil {
		t.Fatalf("insert task failed: %v", err)
	}
}

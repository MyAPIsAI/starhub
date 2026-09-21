package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"starhub/common"
	"starhub/model"
	"starhub/relay"
)

const upstreamKey = "sk-upstream-key-000000000"

func newMockUpstream(t *testing.T) *httptest.Server {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+upstreamKey {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"error":{"message":"Incorrect API key provided","type":"invalid_request_error","code":"invalid_api_key"}}`)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Model         string          `json:"model"`
			Stream        bool            `json:"stream"`
			StreamOptions json.RawMessage `json:"stream_options"`
		}
		_ = json.Unmarshal(body, &req)
		if req.Stream {
			if len(req.StreamOptions) == 0 {
				t.Errorf("relay should inject stream_options.include_usage")
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			flusher := w.(http.Flusher)
			chunks := []string{
				`data: {"id":"mock","choices":[{"index":0,"delta":{"role":"assistant"}}]}`,
				`data: {"id":"mock","choices":[{"index":0,"delta":{"content":"hello "}}]}`,
				`data: {"id":"mock","choices":[{"index":0,"delta":{"content":"world"}}]}`,
				`data: {"id":"mock","choices":[{"index":0,"delta":{}}],"usage":{"prompt_tokens":100,"completion_tokens":50}}`,
				`data: [DONE]`,
			}
			for _, c := range chunks {
				fmt.Fprintf(w, "%s\n\n", c)
				flusher.Flush()
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"id":"mock","object":"chat.completion","model":%q,"choices":[{"index":0,"message":{"role":"assistant","content":"hello world"}}],"usage":{"prompt_tokens":100,"completion_tokens":50}}`, req.Model)
	}))
	t.Cleanup(upstream.Close)
	return upstream
}

type apiResult struct {
	Status int
	Body   string
}

func doRequest(base, method, path, token string, payload any) apiResult {
	var body io.Reader
	if payload != nil {
		b, _ := json.Marshal(payload)
		body = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, base+path, body)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return apiResult{Status: -1, Body: err.Error()}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return apiResult{Status: resp.StatusCode, Body: string(raw)}
}

func TestP0EndToEndRelayFlow(t *testing.T) {
	upstream := newMockUpstream(t)

	if err := model.InitDB(filepath.Join(t.TempDir(), "e2e.db")); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	t.Cleanup(func() { _ = model.CloseDB() })
	if err := model.LoadOptionsToSettings(); err != nil {
		t.Fatalf("LoadOptionsToSettings failed: %v", err)
	}
	if err := model.Seed("123456"); err != nil {
		t.Fatalf("Seed failed: %v", err)
	}

	app := httptest.NewServer(NewRouter())
	t.Cleanup(app.Close)
	base := app.URL

	// 1. root login
	res := doRequest(base, "POST", "/api/user/login", "", map[string]string{"username": "root", "password": "123456"})
	if res.Status != 200 {
		t.Fatalf("login failed: %d %s", res.Status, res.Body)
	}
	var loginResp struct {
		Success bool `json:"success"`
		Data    struct {
			Session string `json:"session"`
			User    struct {
				ID    int64 `json:"id"`
				Quota int64 `json:"quota"`
				Role  int   `json:"role"`
			} `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(res.Body), &loginResp); err != nil || !loginResp.Success {
		t.Fatalf("bad login response: %s", res.Body)
	}
	rootID, session := loginResp.Data.User.ID, loginResp.Data.Session

	res = doRequest(base, "POST", "/api/user/login", "", map[string]string{"username": "root", "password": "wrong"})
	if res.Status != 401 {
		t.Fatalf("wrong password should be 401, got %d", res.Status)
	}

	// 2. set group_ratio (ops setting)
	res = doRequest(base, "PUT", "/api/option/", session, map[string]string{"key": "group_ratio", "value": `{"default":1}`})
	if res.Status != 200 {
		t.Fatalf("set group_ratio failed: %d %s", res.Status, res.Body)
	}
	res = doRequest(base, "PUT", "/api/option/", session, map[string]string{"key": "group_ratio", "value": `bad json`})
	if res.Status != 400 {
		t.Fatalf("invalid ratio JSON should be 400, got %d", res.Status)
	}
	res = doRequest(base, "GET", "/api/option/", session, nil)
	if !strings.Contains(res.Body, "group_ratio") {
		t.Fatalf("options should list group_ratio: %s", res.Body)
	}

	// 3. create channel pointing at mock upstream
	res = doRequest(base, "POST", "/api/channel/", session, map[string]any{
		"name":     "mock-openai",
		"type":     1,
		"key":      upstreamKey,
		"base_url": upstream.URL,
		"models":   "gpt-4o-mini",
		"group":    "default",
	})
	if res.Status != 200 {
		t.Fatalf("create channel failed: %d %s", res.Status, res.Body)
	}
	var channelResp struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(res.Body), &channelResp)
	channelID := channelResp.Data.ID

	res = doRequest(base, "GET", "/api/channel/", session, nil)
	if strings.Contains(res.Body, upstreamKey) {
		t.Fatalf("channel key must be masked in list response: %s", res.Body)
	}
	if !strings.Contains(res.Body, "******") {
		t.Fatalf("channel key should show a mask: %s", res.Body)
	}

	// 4. top up user quota
	res = doRequest(base, "PUT", "/api/user/", session, map[string]any{"id": rootID, "quota": 10000000})
	if res.Status != 200 {
		t.Fatalf("topup failed: %d %s", res.Status, res.Body)
	}
	user, _ := model.GetUserByID(rootID)
	if user.Quota != 10000000 {
		t.Fatalf("user quota should be 10000000, got %d", user.Quota)
	}

	// 5. create token
	res = doRequest(base, "POST", "/api/token/", session, map[string]any{"name": "e2e-key"})
	if res.Status != 200 {
		t.Fatalf("create token failed: %d %s", res.Status, res.Body)
	}
	var tokenResp struct {
		Data struct {
			ID  int64  `json:"id"`
			Key string `json:"key"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(res.Body), &tokenResp); err != nil {
		t.Fatalf("bad token response: %s", res.Body)
	}
	tokenID, apiKey := tokenResp.Data.ID, tokenResp.Data.Key
	if !strings.HasPrefix(apiKey, "sk-") {
		t.Fatalf("token key should start with sk-: %q", apiKey)
	}
	token, _ := model.GetTokenByID(tokenID)
	if token.RemainQuota != common.DefaultTokenQuota {
		t.Fatalf("token should start with default quota, got %d", token.RemainQuota)
	}

	// 6. non-stream relay round trip
	chatBody := map[string]any{
		"model":    "gpt-4o-mini",
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	}
	res = doRequest(base, "POST", "/v1/chat/completions", apiKey, chatBody)
	if res.Status != 200 {
		t.Fatalf("chat completions failed: %d %s", res.Status, res.Body)
	}
	if !strings.Contains(res.Body, "hello world") {
		t.Fatalf("unexpected relay response: %s", res.Body)
	}

	expected := relay.ComputeQuota("gpt-4o-mini", "default", 100, 50)
	if expected <= 0 {
		t.Fatalf("expected quota should be positive, got %d", expected)
	}
	token, _ = model.GetTokenByID(tokenID)
	if token.RemainQuota != common.DefaultTokenQuota-expected {
		t.Fatalf("token remain quota = %d, want %d", token.RemainQuota, common.DefaultTokenQuota-expected)
	}
	channel, _ := model.GetChannelByID(channelID)
	if channel.UsedQuota != expected {
		t.Fatalf("channel used quota = %d, want %d", channel.UsedQuota, expected)
	}
	user, _ = model.GetUserByID(rootID)
	if user.Quota != 10000000-expected || user.UsedQuota != expected {
		t.Fatalf("user quota = %d used = %d, want %d / %d", user.Quota, user.UsedQuota, 10000000-expected, expected)
	}
	logs, total, err := model.ListLogs(rootID, 1, 10, "gpt-4o-mini")
	if err != nil || total != 1 || len(logs) != 1 {
		t.Fatalf("log record missing: total=%d err=%v", total, err)
	}
	if logs[0].PromptTokens != 100 || logs[0].CompletionTokens != 50 || logs[0].Quota != expected {
		t.Fatalf("log fields wrong: %+v", logs[0])
	}

	// 7. stream relay round trip
	streamBody := map[string]any{
		"model":    "gpt-4o-mini",
		"stream":   true,
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	}
	res = doRequest(base, "POST", "/v1/chat/completions", apiKey, streamBody)
	if res.Status != 200 {
		t.Fatalf("stream chat failed: %d %s", res.Status, res.Body)
	}
	if !strings.Contains(res.Body, "data:") || !strings.Contains(res.Body, "[DONE]") ||
		!strings.Contains(res.Body, `"hello "`) || !strings.Contains(res.Body, `"world"`) {
		t.Fatalf("stream response incomplete: %s", res.Body)
	}
	token, _ = model.GetTokenByID(tokenID)
	if token.RemainQuota != common.DefaultTokenQuota-2*expected {
		t.Fatalf("token remain after stream = %d, want %d", token.RemainQuota, common.DefaultTokenQuota-2*expected)
	}
	channel, _ = model.GetChannelByID(channelID)
	if channel.UsedQuota != 2*expected {
		t.Fatalf("channel used after stream = %d, want %d", channel.UsedQuota, 2*expected)
	}

	// 8. GET /v1/models
	res = doRequest(base, "GET", "/v1/models", apiKey, nil)
	if res.Status != 200 || !strings.Contains(res.Body, `"gpt-4o-mini"`) {
		t.Fatalf("models list wrong: %d %s", res.Status, res.Body)
	}

	// 9. log query API
	res = doRequest(base, "GET", "/api/log/?p=1&page_size=5", session, nil)
	if res.Status != 200 || !strings.Contains(res.Body, `"total":2`) {
		t.Fatalf("log API wrong: %d %s", res.Status, res.Body)
	}

	// 10. auth failures
	res = doRequest(base, "POST", "/v1/chat/completions", "sk-not-a-real-key", chatBody)
	if res.Status != 401 || !strings.Contains(res.Body, `"error"`) {
		t.Fatalf("bad key should give 401 envelope, got %d %s", res.Status, res.Body)
	}
	res = doRequest(base, "POST", "/v1/chat/completions", apiKey, map[string]any{"model": "nonexistent-model", "messages": []string{}})
	if res.Status != 503 || !strings.Contains(res.Body, "no_available_channel") {
		t.Fatalf("unknown model should give 503 no_available_channel, got %d %s", res.Status, res.Body)
	}
	tokenAfter, _ := model.GetTokenByID(tokenID)
	if tokenAfter.RemainQuota != token.RemainQuota {
		t.Fatalf("failed requests must not consume quota: %d vs %d", tokenAfter.RemainQuota, token.RemainQuota)
	}

	// 11. insufficient token quota
	res = doRequest(base, "POST", "/api/token/", session, map[string]any{"name": "poor-token", "remain_quota": 5})
	if res.Status != 200 {
		t.Fatalf("create poor token failed: %d %s", res.Status, res.Body)
	}
	var poorResp struct {
		Data struct {
			ID  int64  `json:"id"`
			Key string `json:"key"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(res.Body), &poorResp)
	res = doRequest(base, "POST", "/v1/chat/completions", poorResp.Data.Key, chatBody)
	if res.Status != 403 || !strings.Contains(res.Body, "insufficient_quota") {
		t.Fatalf("insufficient token quota should 403, got %d %s", res.Status, res.Body)
	}

	// 12. upstream auth failure -> passthrough + auto disable
	res = doRequest(base, "POST", "/api/channel/", session, map[string]any{
		"name":     "broken-upstream",
		"type":     1,
		"key":      "sk-wrong-upstream-key",
		"base_url": upstream.URL,
		"models":   "gpt-4o-mini",
		"group":    "default",
		"priority": 10,
	})
	if res.Status != 200 {
		t.Fatalf("create broken channel failed: %d %s", res.Status, res.Body)
	}
	var brokenResp struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(res.Body), &brokenResp)

	res = doRequest(base, "POST", "/v1/chat/completions", apiKey, chatBody)
	if res.Status != 401 || !strings.Contains(res.Body, "Incorrect API key provided") {
		t.Fatalf("upstream 401 should pass through, got %d %s", res.Status, res.Body)
	}
	broken, _ := model.GetChannelByID(brokenResp.Data.ID)
	if broken.Status != common.ChannelStatusAutoDisabled {
		t.Fatalf("broken channel should be auto-disabled, got status %d", broken.Status)
	}
	tokenAfter, _ = model.GetTokenByID(tokenID)
	if tokenAfter.RemainQuota != common.DefaultTokenQuota-2*expected {
		t.Fatalf("refunded request should keep quota at %d, got %d", common.DefaultTokenQuota-2*expected, tokenAfter.RemainQuota)
	}

	// 13. traffic falls back to the healthy channel
	res = doRequest(base, "POST", "/v1/chat/completions", apiKey, chatBody)
	if res.Status != 200 || !strings.Contains(res.Body, "hello world") {
		t.Fatalf("should route to healthy channel after auto-disable, got %d %s", res.Status, res.Body)
	}
	channel, _ = model.GetChannelByID(channelID)
	if channel.UsedQuota != 3*expected {
		t.Fatalf("channel used = %d, want %d", channel.UsedQuota, 3*expected)
	}

	// 14. disabled token is rejected
	_ = model.UpdateTokenFields(tokenID, map[string]any{"status": common.StatusDisabled})
	res = doRequest(base, "POST", "/v1/chat/completions", apiKey, chatBody)
	if res.Status != 401 || !strings.Contains(res.Body, "token_disabled") {
		t.Fatalf("disabled token should 401, got %d %s", res.Status, res.Body)
	}

	// 15. management API requires login
	res = doRequest(base, "GET", "/api/channel/", "", nil)
	if res.Status != 401 {
		t.Fatalf("unauthenticated management call should 401, got %d", res.Status)
	}
}

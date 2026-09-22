package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
	"starhub/relay"
	"starhub/service"
	"starhub/setting"
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

	// Reset rate limiters and settings to defaults (see setupRelayApp for
	// the rationale).
	middleware.ResetRateLimits()
	middleware.ResetTokenRateLimits()
	middleware.ResetModelRateLimits()
	setting.ResetAllToDefaults()

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

	app := httptest.NewServer(middleware.IPRateLimit(NewRouter()))
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

	// 16. token list must never reveal full keys (B10)
	res = doRequest(base, "GET", "/api/token/", session, nil)
	if res.Status != 200 {
		t.Fatalf("token list failed: %d %s", res.Status, res.Body)
	}
	if strings.Contains(res.Body, apiKey) {
		t.Fatalf("token list must not reveal full keys: %s", res.Body)
	}
	if !strings.Contains(res.Body, "******") {
		t.Fatalf("token list should contain masked keys: %s", res.Body)
	}

	// 17. register / login closed loop (A2)
	res = doRequest(base, "POST", "/api/user/register", "", map[string]string{
		"username": "alice",
		"password": "alice-pass",
		"email":    "alice@example.com",
	})
	if res.Status != 200 {
		t.Fatalf("register failed: %d %s", res.Status, res.Body)
	}
	var registerResp struct {
		Data struct {
			ID    int64  `json:"id"`
			Role  int    `json:"role"`
			Email string `json:"email"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(res.Body), &registerResp); err != nil {
		t.Fatalf("bad register response: %s", res.Body)
	}
	aliceID := registerResp.Data.ID
	if registerResp.Data.Role != common.RoleUser || registerResp.Data.Email != "alice@example.com" {
		t.Fatalf("registered user should be a plain user with the given email: %s", res.Body)
	}
	res = doRequest(base, "POST", "/api/user/register", "", map[string]string{"username": "alice", "password": "alice-pass"})
	if res.Status != 409 {
		t.Fatalf("duplicate username should be 409, got %d", res.Status)
	}
	res = doRequest(base, "POST", "/api/user/register", "", map[string]string{"username": "x", "password": "alice-pass"})
	if res.Status != 400 {
		t.Fatalf("short username should be 400, got %d", res.Status)
	}
	res = doRequest(base, "POST", "/api/user/login", "", map[string]string{"username": "alice", "password": "alice-pass"})
	if res.Status != 200 {
		t.Fatalf("alice login failed: %d %s", res.Status, res.Body)
	}
	var aliceLogin struct {
		Data struct {
			Session string `json:"session"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(res.Body), &aliceLogin); err != nil || aliceLogin.Data.Session == "" {
		t.Fatalf("bad alice login response: %s", res.Body)
	}
	aliceSession := aliceLogin.Data.Session

	// 18. self endpoint reflects the fresh account (A4)
	res = doRequest(base, "GET", "/api/user/self", aliceSession, nil)
	if res.Status != 200 || !strings.Contains(res.Body, `"request_count":0`) || !strings.Contains(res.Body, `"quota":0`) {
		t.Fatalf("alice self should show a fresh account: %d %s", res.Status, res.Body)
	}

	// 19. admin tops up the new user (A3 user quota management)
	res = doRequest(base, "PUT", "/api/user/", session, map[string]any{"id": aliceID, "quota": 1000000})
	if res.Status != 200 {
		t.Fatalf("topup alice failed: %d %s", res.Status, res.Body)
	}

	// 20. the registered user relays through an own token
	res = doRequest(base, "POST", "/api/token/", aliceSession, map[string]any{"name": "alice-key"})
	if res.Status != 200 {
		t.Fatalf("alice create token failed: %d %s", res.Status, res.Body)
	}
	var aliceTokenResp struct {
		Data struct {
			Key string `json:"key"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(res.Body), &aliceTokenResp); err != nil || aliceTokenResp.Data.Key == "" {
		t.Fatalf("bad alice token response: %s", res.Body)
	}
	aliceKey := aliceTokenResp.Data.Key
	res = doRequest(base, "POST", "/v1/chat/completions", aliceKey, chatBody)
	if res.Status != 200 || !strings.Contains(res.Body, "hello world") {
		t.Fatalf("alice relay failed: %d %s", res.Status, res.Body)
	}
	res = doRequest(base, "GET", "/api/user/self", aliceSession, nil)
	if res.Status != 200 || !strings.Contains(res.Body, `"request_count":1`) {
		t.Fatalf("alice self should count one request: %d %s", res.Status, res.Body)
	}

	// 21. daily usage aggregation (A4)
	type usageItem struct {
		Date             string `json:"date"`
		Requests         int64  `json:"requests"`
		Quota            int64  `json:"quota"`
		PromptTokens     int64  `json:"prompt_tokens"`
		CompletionTokens int64  `json:"completion_tokens"`
	}
	parseUsage := func(body string) []usageItem {
		var resp struct {
			Data struct {
				Items []usageItem `json:"items"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &resp); err != nil {
			t.Fatalf("bad usage response: %s", body)
		}
		return resp.Data.Items
	}
	today := time.Now().UTC().Format("2006-01-02")

	res = doRequest(base, "GET", "/api/user/usage?days=7", aliceSession, nil)
	if res.Status != 200 {
		t.Fatalf("alice usage failed: %d %s", res.Status, res.Body)
	}
	aliceItems := parseUsage(res.Body)
	if len(aliceItems) != 1 || aliceItems[0].Date != today ||
		aliceItems[0].Requests != 1 || aliceItems[0].Quota != expected {
		t.Fatalf("alice usage wrong: %+v (want 1 request, quota %d)", aliceItems, expected)
	}
	res = doRequest(base, "GET", "/api/user/usage?days=7&user_id=0", aliceSession, nil)
	if res.Status != 200 || len(parseUsage(res.Body)) != 1 {
		t.Fatalf("non-admin must always see only own usage: %s", res.Body)
	}
	res = doRequest(base, "GET", "/api/user/usage?days=7", session, nil)
	if res.Status != 200 {
		t.Fatalf("root usage failed: %d %s", res.Status, res.Body)
	}
	rootItems := parseUsage(res.Body)
	if len(rootItems) != 1 || rootItems[0].Requests != 3 || rootItems[0].Quota != 3*expected {
		t.Fatalf("root usage wrong (want 3 requests, quota %d): %+v", 3*expected, rootItems)
	}
	res = doRequest(base, "GET", "/api/user/usage?days=7&user_id=0", session, nil)
	if res.Status != 200 {
		t.Fatalf("global usage failed: %d %s", res.Status, res.Body)
	}
	globalItems := parseUsage(res.Body)
	if len(globalItems) != 1 || globalItems[0].Requests != 4 || globalItems[0].Quota != 4*expected {
		t.Fatalf("global usage wrong (want 4 requests, quota %d): %+v", 4*expected, globalItems)
	}

	// 22. channel test button writes response_time (B7)
	res = doRequest(base, "POST", "/api/channel/test/"+fmt.Sprint(channelID), session, nil)
	if res.Status != 200 {
		t.Fatalf("channel test failed: %d %s", res.Status, res.Body)
	}
	var channelTestResp struct {
		Data struct {
			OK        bool   `json:"ok"`
			Model     string `json:"model"`
			ElapsedMs int64  `json:"elapsed_ms"`
			Status    int    `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(res.Body), &channelTestResp); err != nil {
		t.Fatalf("bad channel test response: %s", res.Body)
	}
	if !channelTestResp.Data.OK || channelTestResp.Data.Model != "gpt-4o-mini" || channelTestResp.Data.ElapsedMs < 0 {
		t.Fatalf("channel test result wrong: %s", res.Body)
	}
	channel, _ = model.GetChannelByID(channelID)
	if channel.ResponseTime != channelTestResp.Data.ElapsedMs {
		t.Fatalf("channel response_time = %d, want %d", channel.ResponseTime, channelTestResp.Data.ElapsedMs)
	}
	res = doRequest(base, "POST", "/api/channel/test/"+fmt.Sprint(channelID)+"?model=gpt-4o", session, nil)
	if res.Status != 400 {
		t.Fatalf("testing a model outside the channel should be 400, got %d", res.Status)
	}

	// 23. fixing the key and re-testing re-enables the channel (B7)
	res = doRequest(base, "PUT", "/api/channel/", session, map[string]any{"id": brokenResp.Data.ID, "key": upstreamKey})
	if res.Status != 200 {
		t.Fatalf("fix broken channel failed: %d %s", res.Status, res.Body)
	}
	broken, _ = model.GetChannelByID(brokenResp.Data.ID)
	if broken.Status != common.ChannelStatusAutoDisabled {
		t.Fatalf("channel should stay disabled until a test passes, got status %d", broken.Status)
	}
	res = doRequest(base, "POST", "/api/channel/test/"+fmt.Sprint(brokenResp.Data.ID), session, nil)
	if res.Status != 200 || !strings.Contains(res.Body, `"ok":true`) {
		t.Fatalf("re-test after fix should succeed: %d %s", res.Status, res.Body)
	}
	broken, _ = model.GetChannelByID(brokenResp.Data.ID)
	if broken.Status != common.ChannelStatusEnabled {
		t.Fatalf("a passing test should re-enable an auto-disabled channel, got status %d", broken.Status)
	}

	// 24. per-user relay rate limiting (B8)
	middleware.ConfigureRateLimits(middleware.DefaultIPLimitPerMinute, 2)
	middleware.ResetRateLimits()
	t.Cleanup(func() {
		middleware.ConfigureRateLimits(middleware.DefaultIPLimitPerMinute, middleware.DefaultUserLimitPerMinute)
		middleware.ResetRateLimits()
	})
	for i := 0; i < 2; i++ {
		res = doRequest(base, "POST", "/v1/chat/completions", aliceKey, chatBody)
		if res.Status != 200 {
			t.Fatalf("relay call %d under the limit failed: %d %s", i, res.Status, res.Body)
		}
	}
	res = doRequest(base, "POST", "/v1/chat/completions", aliceKey, chatBody)
	if res.Status != 429 || !strings.Contains(res.Body, "rate_limit_error") {
		t.Fatalf("third call should hit the user rate limit: %d %s", res.Status, res.Body)
	}
}

// p1Env bundles the handles a relay-focused e2e test needs.
type p1Env struct {
	base    string
	session string
	rootID  int64
	tokenID int64
	apiKey  string
}

// setupRelayApp boots a fresh app on a fresh DB with a root session and one
// relay token.
func setupRelayApp(t *testing.T) p1Env {
	t.Helper()
	// Reset all rate limiters so token IDs from previous tests don't
	// poison the new test (token IDs are auto-increment from 1, so
	// different tests would share the same rate-limit key otherwise).
	middleware.ResetRateLimits()
	middleware.ResetTokenRateLimits()
	middleware.ResetModelRateLimits()
	// Reset all settings to compiled defaults so dirty state from a
	// previous test (that may have failed before cleanup) doesn't get
	// seeded into the fresh DB.
	setting.ResetAllToDefaults()
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
	app := httptest.NewServer(middleware.IPRateLimit(NewRouter()))
	t.Cleanup(app.Close)

	res := doRequest(app.URL, "POST", "/api/user/login", "",
		map[string]string{"username": "root", "password": "123456"})
	if res.Status != 200 {
		t.Fatalf("login failed: %d %s", res.Status, res.Body)
	}
	var loginResp struct {
		Data struct {
			Session string `json:"session"`
			User    struct {
				ID int64 `json:"id"`
			} `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(res.Body), &loginResp); err != nil || loginResp.Data.Session == "" {
		t.Fatalf("bad login response: %s", res.Body)
	}
	res = doRequest(app.URL, "POST", "/api/token/", loginResp.Data.Session, map[string]any{"name": "p1-key"})
	if res.Status != 200 {
		t.Fatalf("create token failed: %d %s", res.Status, res.Body)
	}
	var tokenResp struct {
		Data struct {
			ID  int64  `json:"id"`
			Key string `json:"key"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(res.Body), &tokenResp); err != nil || tokenResp.Data.Key == "" {
		t.Fatalf("bad token response: %s", res.Body)
	}
	return p1Env{
		base:    app.URL,
		session: loginResp.Data.Session,
		rootID:  loginResp.Data.User.ID,
		tokenID: tokenResp.Data.ID,
		apiKey:  tokenResp.Data.Key,
	}
}

func createChannelID(t *testing.T, env p1Env, payload map[string]any) int64 {
	t.Helper()
	res := doRequest(env.base, "POST", "/api/channel/", env.session, payload)
	if res.Status != 200 {
		t.Fatalf("create channel %v failed: %d %s", payload["name"], res.Status, res.Body)
	}
	var resp struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(res.Body), &resp); err != nil || resp.Data.ID == 0 {
		t.Fatalf("bad create channel response: %s", res.Body)
	}
	return resp.Data.ID
}

func TestP1RelayFailoverRefundAndTrace(t *testing.T) {
	fiveHundred := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error":{"message":"upstream exploded","type":"server_error","code":"internal"}}`)
	}))
	t.Cleanup(fiveHundred.Close)

	badRequest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"error":{"message":"max_tokens is too large","type":"invalid_request_error","code":"max_tokens"}}`)
	}))
	t.Cleanup(badRequest.Close)

	var healthyHits int32
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+upstreamKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		atomic.AddInt32(&healthyHits, 1)
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Model string `json:"model"`
		}
		_ = json.Unmarshal(body, &req)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"id":"mock","object":"chat.completion","model":%q,"choices":[{"index":0,"message":{"role":"assistant","content":"hello world"}}],"usage":{"prompt_tokens":100,"completion_tokens":50}}`, req.Model)
	}))
	t.Cleanup(healthy.Close)

	// a listener that is closed right away: dialing it fails immediately
	deadListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("dead listener failed: %v", err)
	}
	deadURL := "http://" + deadListener.Addr().String()
	_ = deadListener.Close()

	env := setupRelayApp(t)
	chat := func(model string) apiResult {
		return doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey,
			map[string]any{"model": model, "messages": []map[string]string{{"role": "user", "content": "hi"}}})
	}

	ch500 := createChannelID(t, env, map[string]any{
		"name": "always-500", "key": upstreamKey, "base_url": fiveHundred.URL,
		"models": "gpt-fail,gpt-4o-mini", "priority": 30,
	})
	chBad := createChannelID(t, env, map[string]any{
		"name": "bad-request", "key": upstreamKey, "base_url": badRequest.URL,
		"models": "gpt-bad", "priority": 20,
	})
	chOK := createChannelID(t, env, map[string]any{
		"name": "healthy", "key": upstreamKey, "base_url": healthy.URL,
		"models": "gpt-4o-mini,gpt-bad,gpt-net", "priority": 10,
	})
	chDead := createChannelID(t, env, map[string]any{
		"name": "dead-port", "key": upstreamKey, "base_url": deadURL,
		"models": "gpt-net", "priority": 40,
	})

	// 1. a 5xx upstream transparently fails over to the next priority tier
	res := chat("gpt-4o-mini")
	if res.Status != 200 || !strings.Contains(res.Body, "hello world") {
		t.Fatalf("5xx failover should reach the healthy channel: %d %s", res.Status, res.Body)
	}
	logs, total, err := model.ListLogs(env.rootID, 1, 10, "gpt-4o-mini")
	if err != nil || total != 1 || len(logs) != 1 {
		t.Fatalf("failover log missing: total=%d err=%v", total, err)
	}
	if want := fmt.Sprintf("%d->%d", ch500, chOK); logs[0].UsedChannels != want {
		t.Fatalf("retry trace = %q, want %q", logs[0].UsedChannels, want)
	}
	c500, _ := model.GetChannelByID(ch500)
	if c500.Status != common.ChannelStatusEnabled {
		t.Fatalf("a transient 5xx must not auto-disable the channel, got status %d", c500.Status)
	}
	expected1 := relay.ComputeQuota("gpt-4o-mini", "default", 100, 50)
	token, _ := model.GetTokenByID(env.tokenID)
	if token.RemainQuota != common.DefaultTokenQuota-expected1 {
		t.Fatalf("token quota after failover = %d, want %d", token.RemainQuota, common.DefaultTokenQuota-expected1)
	}

	// 2. a dead upstream (network error) also fails over
	res = chat("gpt-net")
	if res.Status != 200 || !strings.Contains(res.Body, "hello world") {
		t.Fatalf("network failure should fail over: %d %s", res.Status, res.Body)
	}
	logs, _, _ = model.ListLogs(env.rootID, 1, 10, "gpt-net")
	if len(logs) != 1 || logs[0].UsedChannels != fmt.Sprintf("%d->%d", chDead, chOK) {
		t.Fatalf("dead-port retry trace wrong: %+v", logs)
	}
	deadCh, _ := model.GetChannelByID(chDead)
	if deadCh.Status != common.ChannelStatusEnabled {
		t.Fatalf("network failure must not auto-disable the channel, got status %d", deadCh.Status)
	}
	expected2 := relay.ComputeQuota("gpt-net", "default", 100, 50)

	// 3. every attempt fails: the request 502s and the pre-payment is refunded
	quotaAfterSuccesses := common.DefaultTokenQuota - expected1 - expected2
	userBefore, _ := model.GetUserByID(env.rootID)
	res = chat("gpt-fail")
	if res.Status != 502 || !strings.Contains(res.Body, "upstream_unavailable") ||
		!strings.Contains(res.Body, "returned 500") {
		t.Fatalf("exhausted channels should 502 with the last error, got %d %s", res.Status, res.Body)
	}
	tokenAfter, _ := model.GetTokenByID(env.tokenID)
	if tokenAfter.RemainQuota != quotaAfterSuccesses {
		t.Fatalf("all-failed request must be refunded: %d, want %d", tokenAfter.RemainQuota, quotaAfterSuccesses)
	}
	userAfter, _ := model.GetUserByID(env.rootID)
	if userAfter.Quota != userBefore.Quota {
		t.Fatalf("all-failed request must refund user quota: %d vs %d", userAfter.Quota, userBefore.Quota)
	}
	if _, total, _ := model.ListLogs(env.rootID, 1, 10, "gpt-fail"); total != 0 {
		t.Fatalf("failed relay must not write usage logs, got %d rows", total)
	}
	c500, _ = model.GetChannelByID(ch500)
	if c500.Status != common.ChannelStatusEnabled {
		t.Fatalf("5xx-only channel must stay enabled for probes to decide, got status %d", c500.Status)
	}

	// 4. a 4xx upstream is passed through and never retried on other channels
	hitsBefore := atomic.LoadInt32(&healthyHits)
	res = chat("gpt-bad")
	if res.Status != 400 || !strings.Contains(res.Body, "max_tokens is too large") {
		t.Fatalf("upstream 400 should pass through unchanged: %d %s", res.Status, res.Body)
	}
	if atomic.LoadInt32(&healthyHits) != hitsBefore {
		t.Fatalf("4xx must not be retried on another channel (hits %d -> %d)",
			hitsBefore, atomic.LoadInt32(&healthyHits))
	}
	tokenAfter, _ = model.GetTokenByID(env.tokenID)
	if tokenAfter.RemainQuota != quotaAfterSuccesses {
		t.Fatalf("4xx request must be refunded: %d, want %d", tokenAfter.RemainQuota, quotaAfterSuccesses)
	}
	if _, total, _ := model.ListLogs(env.rootID, 1, 10, "gpt-bad"); total != 0 {
		t.Fatalf("4xx relay must not write usage logs, got %d rows", total)
	}
	cBad, _ := model.GetChannelByID(chBad)
	if cBad.Status != common.ChannelStatusEnabled {
		t.Fatalf("a plain 400 must not auto-disable the channel, got status %d", cBad.Status)
	}
}

func TestP1BackgroundChannelTestRecovers(t *testing.T) {
	upstream := newMockUpstream(t)
	env := setupRelayApp(t)
	chat := func(model string) apiResult {
		return doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey,
			map[string]any{"model": model, "messages": []map[string]string{{"role": "user", "content": "hi"}}})
	}

	// the relay auto-disables a channel whose upstream key is broken
	chFix := createChannelID(t, env, map[string]any{
		"name": "to-recover", "key": "sk-wrong-key", "base_url": upstream.URL,
		"models": "gpt-4o-mini", "priority": 10,
	})
	res := chat("gpt-4o-mini")
	if res.Status != 401 || !strings.Contains(res.Body, "Incorrect API key provided") {
		t.Fatalf("broken upstream key should pass 401 through: %d %s", res.Status, res.Body)
	}
	token, _ := model.GetTokenByID(env.tokenID)
	if token.RemainQuota != common.DefaultTokenQuota {
		t.Fatalf("401 request must be refunded, got %d", token.RemainQuota)
	}

	// the admin repairs the key; the channel stays disabled until probed
	res = doRequest(env.base, "PUT", "/api/channel/", env.session,
		map[string]any{"id": chFix, "key": upstreamKey})
	if res.Status != 200 {
		t.Fatalf("fix channel key failed: %d %s", res.Status, res.Body)
	}
	cFix, _ := model.GetChannelByID(chFix)
	if cFix.Status != common.ChannelStatusAutoDisabled {
		t.Fatalf("channel should stay auto-disabled until a probe passes, got %d", cFix.Status)
	}

	// a still-broken channel plus a manually disabled one
	chBad := createChannelID(t, env, map[string]any{
		"name": "still-bad", "key": "sk-still-wrong", "base_url": upstream.URL,
		"models": "gpt-4o-mini", "priority": 5,
	})
	chManual := createChannelID(t, env, map[string]any{
		"name": "admin-off", "key": upstreamKey, "base_url": upstream.URL,
		"models": "gpt-4o-mini", "priority": 0, "status": common.ChannelStatusManuallyDisabled,
	})

	// the background health checker reconciles everything in one sweep
	service.RunChannelTestOnce(context.Background())

	cFix, _ = model.GetChannelByID(chFix)
	if cFix.Status != common.ChannelStatusEnabled {
		t.Fatalf("a passing probe should re-enable the channel, got %d", cFix.Status)
	}
	cBad, _ := model.GetChannelByID(chBad)
	if cBad.Status != common.ChannelStatusAutoDisabled {
		t.Fatalf("a failing probe should auto-disable the channel, got %d", cBad.Status)
	}
	cManual, _ := model.GetChannelByID(chManual)
	if cManual.Status != common.ChannelStatusManuallyDisabled {
		t.Fatalf("manually disabled channels must be left alone, got %d", cManual.Status)
	}

	// traffic flows again without any manual action
	res = chat("gpt-4o-mini")
	if res.Status != 200 || !strings.Contains(res.Body, "hello world") {
		t.Fatalf("recovered channel should serve traffic: %d %s", res.Status, res.Body)
	}
}

func TestP1MultiKeyChannelKeyIsolation(t *testing.T) {
	goodKeys := map[string]bool{"sk-good-1": true, "sk-good-2": true}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		auth := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !goodKeys[auth] {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"error":{"message":"Incorrect API key provided","type":"invalid_request_error","code":"invalid_api_key"}}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"id":"mock","object":"chat.completion","model":"gpt-4o-mini","choices":[{"index":0,"message":{"role":"assistant","content":"hello world"}}],"usage":{"prompt_tokens":100,"completion_tokens":50}}`)
	}))
	t.Cleanup(upstream.Close)

	env := setupRelayApp(t)
	model.ResetMultiKeyCounters()
	chat := func(model string) apiResult {
		return doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey,
			map[string]any{"model": model, "messages": []map[string]string{{"role": "user", "content": "hi"}}})
	}

	ch := createChannelID(t, env, map[string]any{
		"name": "multi-key", "key": "sk-bad-key-000001\nsk-good-1\nsk-good-2",
		"key_mode": "round_robin", "base_url": upstream.URL, "models": "gpt-4o-mini",
	})

	// round robin starts at the first (broken) key: the request 401s and only
	// that key is disabled, the channel keeps serving
	res := chat("gpt-4o-mini")
	if res.Status != 401 || !strings.Contains(res.Body, "Incorrect API key provided") {
		t.Fatalf("broken key should pass 401 through: %d %s", res.Status, res.Body)
	}
	got, _ := model.GetChannelByID(ch)
	if got.Status != common.ChannelStatusEnabled {
		t.Fatalf("one broken key must not disable the whole channel, got status %d", got.Status)
	}
	if st := got.KeyStatuses(); len(st) != 3 || st[0] != common.StatusDisabled ||
		st[1] != common.StatusEnabled || st[2] != common.StatusEnabled {
		t.Fatalf("per-key statuses = %v, want only key 0 disabled", st)
	}

	// the next requests skip the dead key and succeed through the good ones
	res = chat("gpt-4o-mini")
	if res.Status != 200 || !strings.Contains(res.Body, "hello world") {
		t.Fatalf("remaining keys should serve traffic: %d %s", res.Status, res.Body)
	}
	res = chat("gpt-4o-mini")
	if res.Status != 200 || !strings.Contains(res.Body, "hello world") {
		t.Fatalf("round robin should alternate the good keys: %d %s", res.Status, res.Body)
	}
	expected := relay.ComputeQuota("gpt-4o-mini", "default", 100, 50)
	token, _ := model.GetTokenByID(env.tokenID)
	if token.RemainQuota != common.DefaultTokenQuota-2*expected {
		t.Fatalf("token quota = %d, want %d (401 refunded, 2 successes charged)",
			token.RemainQuota, common.DefaultTokenQuota-2*expected)
	}

	// the channel API exposes the multi-key state but never the raw keys
	res = doRequest(env.base, "GET", "/api/channel/"+fmt.Sprint(ch), env.session, nil)
	if res.Status != 200 || !strings.Contains(res.Body, `"key_mode":"round_robin"`) ||
		!strings.Contains(res.Body, `"key_count":3`) ||
		!strings.Contains(res.Body, `"multi_key_status":[2,1,1]`) {
		t.Fatalf("channel API should expose multi-key state: %s", res.Body)
	}
	if strings.Contains(res.Body, "sk-good-1") || strings.Contains(res.Body, "sk-bad-key-000001") {
		t.Fatalf("channel API must never reveal raw keys: %s", res.Body)
	}
}

// ---------------------------------------------------------------------------
// P2: billing integrity
// ---------------------------------------------------------------------------

// newP2Upstream returns a mock upstream with configurable usage payload and
// response delay. usageJSON is spliced into the standard 100/50 usage field;
// an empty streamUsageJSON makes the stream response omit the usage chunk.
func newP2Upstream(t *testing.T, usageJSON, streamUsageJSON string, delay time.Duration) *httptest.Server {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+upstreamKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if delay > 0 {
			time.Sleep(delay)
		}
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Model  string `json:"model"`
			Stream bool   `json:"stream"`
		}
		_ = json.Unmarshal(body, &req)
		if req.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			flusher := w.(http.Flusher)
			chunks := []string{
				`data: {"id":"mock","choices":[{"index":0,"delta":{"role":"assistant"}}]}`,
				`data: {"id":"mock","choices":[{"index":0,"delta":{"content":"hello "}}]}`,
				`data: {"id":"mock","choices":[{"index":0,"delta":{"content":"world"}}]}`,
			}
			if streamUsageJSON != "" {
				chunks = append(chunks, `data: {"id":"mock","choices":[{"index":0,"delta":{}}],"usage":`+streamUsageJSON+`}`)
			}
			chunks = append(chunks, `data: [DONE]`)
			for _, c := range chunks {
				fmt.Fprintf(w, "%s\n\n", c)
				flusher.Flush()
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"id":"mock","object":"chat.completion","model":%q,"choices":[{"index":0,"message":{"role":"assistant","content":"hello world"}}],"usage":%s}`,
			req.Model, usageJSON)
	}))
	t.Cleanup(upstream.Close)
	return upstream
}

func setOption(t *testing.T, env p1Env, key, value string) {
	t.Helper()
	res := doRequest(env.base, "PUT", "/api/option/", env.session, map[string]string{"key": key, "value": value})
	if res.Status != 200 {
		t.Fatalf("set option %s failed: %d %s", key, res.Status, res.Body)
	}
}

func TestP2MultiplierValidationRejectsBeforeBilling(t *testing.T) {
	upstream := newP2Upstream(t, `{"prompt_tokens":100,"completion_tokens":50}`, `{"prompt_tokens":100,"completion_tokens":50}`, 0)
	env := setupRelayApp(t)
	createChannelID(t, env, map[string]any{
		"name": "mock", "key": upstreamKey, "base_url": upstream.URL, "models": "gpt-4o-mini",
	})

	invalid := []map[string]any{
		{"model": "gpt-4o-mini", "messages": []map[string]string{{"role": "user", "content": "hi"}}, "max_tokens": common.MaxChatMaxTokens + 1},
		{"model": "gpt-4o-mini", "messages": []map[string]string{{"role": "user", "content": "hi"}}, "max_tokens": -1},
		{"model": "gpt-4o-mini", "messages": []map[string]string{{"role": "user", "content": "hi"}}, "n": common.MaxChatChoices + 1},
		{"model": "gpt-4o-mini", "messages": []map[string]string{{"role": "user", "content": "hi"}}, "n": -2},
		{"model": "gpt-4o-mini", "messages": []map[string]string{{"role": "user", "content": "hi"}}, "max_completion_tokens": 1 << 40},
	}
	for _, payload := range invalid {
		res := doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, payload)
		if res.Status != 400 || !strings.Contains(res.Body, "invalid_request_body") {
			t.Fatalf("out-of-range multiplier should 400 before billing: %d %s (payload n=%v max=%v)",
				res.Status, res.Body, payload["n"], payload["max_tokens"])
		}
	}
	token, _ := model.GetTokenByID(env.tokenID)
	if token.RemainQuota != common.DefaultTokenQuota {
		t.Fatalf("rejected requests must not touch quota, got %d", token.RemainQuota)
	}

	// in-bounds multipliers pass the gate and bill normally
	res := doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, map[string]any{
		"model": "gpt-4o-mini", "max_tokens": 500, "n": 2,
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	})
	if res.Status != 200 {
		t.Fatalf("valid multipliers should relay fine: %d %s", res.Status, res.Body)
	}
	expected := relay.ComputeQuota("gpt-4o-mini", "default", 100, 50)
	token, _ = model.GetTokenByID(env.tokenID)
	if token.RemainQuota != common.DefaultTokenQuota-expected {
		t.Fatalf("token quota after valid call = %d, want %d", token.RemainQuota, common.DefaultTokenQuota-expected)
	}
}

func TestP2PerCallAndExpressionPricing(t *testing.T) {
	upstream := newP2Upstream(t, `{"prompt_tokens":100,"completion_tokens":50}`, `{"prompt_tokens":100,"completion_tokens":50}`, 0)
	env := setupRelayApp(t)
	createChannelID(t, env, map[string]any{
		"name": "mock", "key": upstreamKey, "base_url": upstream.URL, "models": "gpt-4o-mini",
	})
	chat := func() apiResult {
		return doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey,
			map[string]any{"model": "gpt-4o-mini", "messages": []map[string]string{{"role": "user", "content": "hi"}}})
	}
	remain := func() int64 {
		token, _ := model.GetTokenByID(env.tokenID)
		return token.RemainQuota
	}

	// per-call fixed price: 0.01 USD = 5000 quota regardless of usage
	setOption(t, env, "model_price", `{"gpt-4o-mini":0.01}`)
	if res := chat(); res.Status != 200 {
		t.Fatalf("per-call relay failed: %d %s", res.Status, res.Body)
	}
	if got := remain(); got != common.DefaultTokenQuota-5000 {
		t.Fatalf("per-call billing = %d, want %d", got, common.DefaultTokenQuota-5000)
	}

	// tiered expression wins over the fixed price: prompt 100 hits tier 1
	// (0.001 USD = 500 quota)
	setOption(t, env, "model_price_expr", `{"gpt-4o-mini":"prompt_tokens <= 100 ? 0.001 : 0.01"}`)
	if res := chat(); res.Status != 200 {
		t.Fatalf("expression relay failed: %d %s", res.Status, res.Body)
	}
	if got := remain(); got != common.DefaultTokenQuota-5000-500 {
		t.Fatalf("expression billing = %d, want %d", got, common.DefaultTokenQuota-5500)
	}

	// removing both restores token billing
	setOption(t, env, "model_price", `{}`)
	setOption(t, env, "model_price_expr", `{}`)
	if res := chat(); res.Status != 200 {
		t.Fatalf("token-billing relay failed: %d %s", res.Status, res.Body)
	}
	expected := relay.ComputeQuota("gpt-4o-mini", "default", 100, 50)
	if got := remain(); got != common.DefaultTokenQuota-5500-expected {
		t.Fatalf("token billing = %d, want %d", got, common.DefaultTokenQuota-5500-expected)
	}

	// invalid pricing options are rejected by the option API
	res := doRequest(env.base, "PUT", "/api/option/", env.session,
		map[string]string{"key": "model_price_expr", "value": `{"gpt-4o-mini":"hack(1)"}`})
	if res.Status != 400 {
		t.Fatalf("non-whitelisted expression must be rejected: %d %s", res.Status, res.Body)
	}
}

func TestP2SettlementUsesCacheTokenDetail(t *testing.T) {
	upstream := newP2Upstream(t,
		`{"prompt_tokens":100,"completion_tokens":50,"prompt_tokens_details":{"cached_tokens":80}}`,
		`{"prompt_tokens":100,"completion_tokens":50,"prompt_tokens_details":{"cached_tokens":80}}`, 0)
	env := setupRelayApp(t)
	createChannelID(t, env, map[string]any{
		"name": "mock", "key": upstreamKey, "base_url": upstream.URL, "models": "gpt-4o-mini",
	})

	chat := func() apiResult {
		return doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey,
			map[string]any{"model": "gpt-4o-mini", "messages": []map[string]string{{"role": "user", "content": "hi"}}})
	}
	expected := relay.ComputeUsageQuota(relay.NewPriceData("gpt-4o-mini", "default"),
		&relay.Usage{PromptTokens: 100, CompletionTokens: 50,
			PromptTokensDetails: &relay.PromptTokensDetails{CachedTokens: 80}})
	if expected != 20 {
		t.Fatalf("cached-detail settlement math changed: %d, want 20", expected)
	}
	plain := relay.ComputeQuota("gpt-4o-mini", "default", 100, 50)

	if res := chat(); res.Status != 200 {
		t.Fatalf("cached-token relay failed: %d %s", res.Status, res.Body)
	}
	token, _ := model.GetTokenByID(env.tokenID)
	if token.RemainQuota != common.DefaultTokenQuota-expected {
		t.Fatalf("cached-token billing = %d, want %d (plain %d)", token.RemainQuota,
			common.DefaultTokenQuota-expected, plain)
	}

	// the same detail applies to streaming usage chunks
	streamRes := doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey,
		map[string]any{"model": "gpt-4o-mini", "stream": true,
			"messages": []map[string]string{{"role": "user", "content": "hi"}}})
	if streamRes.Status != 200 {
		t.Fatalf("cached-token stream relay failed: %d %s", streamRes.Status, streamRes.Body)
	}
	token, _ = model.GetTokenByID(env.tokenID)
	if token.RemainQuota != common.DefaultTokenQuota-2*expected {
		t.Fatalf("cached-token stream billing = %d, want %d", token.RemainQuota, common.DefaultTokenQuota-2*expected)
	}
}

func TestP2StreamWithoutUsageStillBills(t *testing.T) {
	// upstream ignores stream_options.include_usage: no usage chunk, no
	// usage field — the relay must fall back to local estimates
	upstream := newP2Upstream(t, `null`, ``, 0)
	env := setupRelayApp(t)
	createChannelID(t, env, map[string]any{
		"name": "mock", "key": upstreamKey, "base_url": upstream.URL, "models": "gpt-4o-mini",
	})

	chatBody := map[string]any{"model": "gpt-4o-mini",
		"messages": []map[string]string{{"role": "user", "content": "hi"}}}
	bodyBytes, _ := json.Marshal(chatBody)
	est := relay.EstimateUsage(bodyBytes, "hello world")
	expected := relay.ComputeQuota("gpt-4o-mini", "default", est.PromptTokens, est.CompletionTokens)
	if expected <= 0 {
		t.Fatalf("estimated billing should be positive, got %d (usage %+v)", expected, est)
	}

	res := doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, chatBody)
	if res.Status != 200 || !strings.Contains(res.Body, "hello world") {
		t.Fatalf("usage-less non-stream relay failed: %d %s", res.Status, res.Body)
	}
	streamRes := doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey,
		map[string]any{"model": "gpt-4o-mini", "stream": true,
			"messages": []map[string]string{{"role": "user", "content": "hi"}}})
	if streamRes.Status != 200 || !strings.Contains(streamRes.Body, "[DONE]") {
		t.Fatalf("usage-less stream relay failed: %d %s", streamRes.Status, streamRes.Body)
	}

	token, _ := model.GetTokenByID(env.tokenID)
	if token.RemainQuota != common.DefaultTokenQuota-2*expected {
		t.Fatalf("usage-less relay must still bill the estimate: %d, want %d",
			token.RemainQuota, common.DefaultTokenQuota-2*expected)
	}
	logs, total, err := model.ListLogs(env.rootID, 1, 10, "gpt-4o-mini")
	if err != nil || total != 2 {
		t.Fatalf("usage-less relay must leave audit logs: total=%d err=%v", total, err)
	}
	for _, l := range logs {
		if l.Quota != expected || l.PromptTokens != est.PromptTokens || l.CompletionTokens != est.CompletionTokens {
			t.Fatalf("estimated log row wrong: %+v (want quota %d tokens %d/%d)",
				l, expected, est.PromptTokens, est.CompletionTokens)
		}
	}
}

func TestP2ConcurrentPreConsumeNoOverdraft(t *testing.T) {
	// slow upstream so every concurrent request holds its pre-payment while
	// the others try to reserve theirs
	upstream := newP2Upstream(t, `{"prompt_tokens":100,"completion_tokens":50}`,
		`{"prompt_tokens":100,"completion_tokens":50}`, 600*time.Millisecond)
	env := setupRelayApp(t)
	createChannelID(t, env, map[string]any{
		"name": "slow-mock", "key": upstreamKey, "base_url": upstream.URL, "models": "gpt-4o-mini",
	})

	chatBody := map[string]any{"model": "gpt-4o-mini", "max_tokens": 1000,
		"messages": []map[string]string{{"role": "user", "content": "hi"}}}
	bodyBytes, _ := json.Marshal(chatBody)
	pre := relay.EstimatePreConsumeQuota("gpt-4o-mini", "default", bodyBytes)
	if pre <= 0 {
		t.Fatalf("pre-charge estimate should be positive, got %d", pre)
	}
	// budget covers 2 full pre-charges plus half of a third: exactly two of
	// the four concurrent requests may pass
	budget := 2*pre + pre/2

	res := doRequest(env.base, "PUT", "/api/user/", env.session,
		map[string]any{"id": env.rootID, "quota": budget})
	if res.Status != 200 {
		t.Fatalf("tighten user quota failed: %d %s", res.Status, res.Body)
	}
	res = doRequest(env.base, "POST", "/api/token/", env.session,
		map[string]any{"name": "concurrent", "remain_quota": budget})
	if res.Status != 200 {
		t.Fatalf("create concurrent token failed: %d %s", res.Status, res.Body)
	}
	var tokenResp struct {
		Data struct {
			ID  int64  `json:"id"`
			Key string `json:"key"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(res.Body), &tokenResp); err != nil || tokenResp.Data.Key == "" {
		t.Fatalf("bad token response: %s", res.Body)
	}
	limitedKey := tokenResp.Data.Key

	const workers = 4
	results := make([]apiResult, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = doRequest(env.base, "POST", "/v1/chat/completions", limitedKey, chatBody)
		}(i)
	}
	wg.Wait()

	var ok, rejected, other int
	for _, r := range results {
		switch {
		case r.Status == 200:
			ok++
		case r.Status == 403 && strings.Contains(r.Body, "insufficient_quota"):
			rejected++
		default:
			other++
			t.Logf("unexpected concurrent result: %d %s", r.Status, r.Body)
		}
	}
	if other > 0 {
		t.Fatalf("%d concurrent requests produced unexpected statuses", other)
	}
	if ok != 2 || rejected != 2 {
		t.Fatalf("concurrent overdraft: %d passed, %d rejected (pre=%d budget=%d), want 2/2",
			ok, rejected, pre, budget)
	}

	// after settlement the books balance: each success cost the real usage
	actual := relay.ComputeQuota("gpt-4o-mini", "default", 100, 50)
	token, _ := model.GetTokenByID(tokenResp.Data.ID)
	if token.RemainQuota != budget-2*actual {
		t.Fatalf("token balance after concurrency = %d, want %d", token.RemainQuota, budget-2*actual)
	}
	user, _ := model.GetUserByID(env.rootID)
	if user.Quota != budget-2*actual {
		t.Fatalf("user balance after concurrency = %d, want %d", user.Quota, budget-2*actual)
	}
	if user.Quota < 0 || token.RemainQuota < 0 {
		t.Fatal("balances must never go negative")
	}
	if user.UsedQuota != 2*actual || token.UsedQuota != 2*actual {
		t.Fatalf("used quota after concurrency: user %d token %d, want %d",
			user.UsedQuota, token.UsedQuota, 2*actual)
	}
	if _, total, _ := model.ListLogs(env.rootID, 1, 10, "gpt-4o-mini"); total != 2 {
		t.Fatalf("both successes must be logged, got %d rows", total)
	}
}

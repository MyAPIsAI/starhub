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
	"time"

	"starhub/common"
	"starhub/middleware"
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

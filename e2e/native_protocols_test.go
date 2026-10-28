package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"starhub/middleware"
	"starhub/model"
)

func TestNativeProtocolsUseRegisteredAuthRateAndBillingChain(t *testing.T) {
	for _, tc := range []struct {
		protocol, path, body, response string
		kind                           int
	}{
		{"OpenAI", "/v1/responses", `{"model":"native-test","input":"hello","max_output_tokens":16}`, `{"id":"resp_test","status":"completed","usage":{"input_tokens":20,"output_tokens":10}}`, 1},
		{"Gemini", "/v1beta/models/native-test:generateContent", `{"contents":[{"parts":[{"text":"hello"}]}],"generationConfig":{"maxOutputTokens":16}}`, `{"candidates":[{"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":20,"candidatesTokenCount":10}}`, 25},
	} {
		t.Run(tc.protocol, func(t *testing.T) {
			env := setupRelayApp(t)
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path != tc.path {
					t.Errorf("unexpected native path: %s", r.URL)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, tc.response)
			}))
			defer upstream.Close()
			createChannelID(t, env, map[string]any{"name": "native", "type": tc.kind, "key": "supplier-key", "base_url": upstream.URL, "models": "native-test"})
			send := func(key string) (int, []byte) {
				t.Helper()
				req, _ := http.NewRequest("POST", env.base+tc.path, strings.NewReader(tc.body))
				req.Header.Set("Content-Type", "application/json")
				if tc.protocol == "Gemini" {
					req.Header.Set("x-goog-api-key", key)
				} else {
					req.Header.Set("Authorization", "Bearer "+key)
				}
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				defer resp.Body.Close()
				raw, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Fatal(err)
				}
				return resp.StatusCode, raw
			}
			if status, body := send("invalid"); status != 401 {
				t.Fatalf("route bypassed auth: %d %s", status, body)
			}
			middleware.ConfigureRateLimits(middleware.DefaultIPLimitPerMinute, 1)
			middleware.ResetRateLimits()
			t.Cleanup(func() {
				middleware.ConfigureRateLimits(middleware.DefaultIPLimitPerMinute, middleware.DefaultUserLimitPerMinute)
				middleware.ResetRateLimits()
			})
			if status, body := send(env.apiKey); status != 200 || string(body) != tc.response {
				t.Fatalf("native registered route: %d %s", status, body)
			}
			if status, body := send(env.apiKey); status != 429 {
				t.Fatalf("native rate limit bypassed: %d %s", status, body)
			} else if tc.protocol == "Gemini" {
				var result struct {
					Error struct {
						Code   int    `json:"code"`
						Status string `json:"status"`
					} `json:"error"`
				}
				if err := json.Unmarshal(body, &result); err != nil || result.Error.Code != 429 || result.Error.Status != "RESOURCE_EXHAUSTED" {
					t.Fatalf("rate error is not native: %s %v", body, err)
				}
			}
			logs, count, err := model.ListLogs(env.rootID, 1, 10, "")
			if err != nil || count != 1 || calls.Load() != 1 || logs[0].Protocol != tc.protocol || logs[0].PromptTokens != 20 || logs[0].CompletionTokens != 10 || logs[0].Quota <= 0 {
				t.Fatalf("native billing chain lost: %+v count=%d calls=%d err=%v", logs, count, calls.Load(), err)
			}
		})
	}
}

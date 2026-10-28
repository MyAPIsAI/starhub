package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"starhub/common"
	"starhub/model"
	"starhub/relay"
	"starhub/relay/channel"
)

func TestInFlightChatProtocolsKeepCapturedPricesAcrossAtomicPriceEdits(t *testing.T) {
	for _, protocol := range []string{"openai", "anthropic", "responses", "gemini"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", protocol, stream), func(t *testing.T) {
				env := setupRelayApp(t)
				update := func(version int64, input, completion string) {
					t.Helper()
					response := doRequest(env.base, "PUT", "/api/option/pricing", env.session, map[string]any{"expected_version": version, "options": []model.Option{{Key: "model_ratio", Value: `{"gpt-4o":` + input + `}`}, {Key: "completion_ratio", Value: `{"gpt-4o":` + completion + `}`}}})
					if response.Status != 200 {
						t.Fatalf("atomic pricing update: %d %s", response.Status, response.Body)
					}
				}
				update(0, "2", "2")
				usage := &relay.Usage{PromptTokens: 100, CompletionTokens: 20}
				original := relay.ComputeUsageQuota(relay.NewPriceData("gpt-4o", "default"), usage)
				if original != 140 {
					t.Fatalf("fixture price=%d, want140", original)
				}
				started, release := make(chan struct{}, 1), make(chan struct{})
				var releaseOnce sync.Once
				response := `{"id":"fixture","object":"chat.completion","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":100,"completion_tokens":20}}`
				if stream {
					response = "data: {\"id\":\"fixture\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"}}]}\n\ndata: {\"id\":\"fixture\",\"choices\":[],\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":20}}\n\ndata: [DONE]\n\n"
				}
				if protocol == "responses" {
					response = `{"id":"resp_fixture","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hello"}]}],"usage":{"input_tokens":100,"output_tokens":20}}`
					if stream {
						response = "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":" + response + "}\n\n"
					}
				}
				if protocol == "gemini" {
					response = `{"candidates":[{"content":{"role":"model","parts":[{"text":"hello"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":20}}`
					if stream {
						response = "data: " + response + "\n\n"
					}
				}
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					select {
					case started <- struct{}{}:
					default:
					}
					select {
					case <-release:
					case <-r.Context().Done():
						return
					}
					kind := "application/json"
					if stream {
						kind = "text/event-stream"
					}
					w.Header().Set("Content-Type", kind)
					_, _ = io.WriteString(w, response)
				}))
				defer upstream.Close()
				defer releaseOnce.Do(func() { close(release) })
				kind := channel.ChannelTypeOpenAI
				if protocol == "gemini" {
					kind = channel.ChannelTypeGemini
				}
				if err := model.InsertChannel(&model.Channel{Name: "Frozen pricing fixture", Type: kind, Key: "synthetic-local", BaseURL: upstream.URL, Models: "gpt-4o", Group: "default", Status: common.ChannelStatusEnabled}); err != nil {
					t.Fatal(err)
				}
				path := "/v1/chat/completions"
				payload := map[string]any{"model": "gpt-4o", "messages": []map[string]string{{"role": "user", "content": "hello"}}, "max_tokens": 16, "stream": stream}
				if protocol == "anthropic" {
					path = "/v1/messages"
				}
				if protocol == "responses" {
					path = "/v1/responses"
					payload = map[string]any{"model": "gpt-4o", "input": "hello", "max_output_tokens": 16, "stream": stream}
				}
				if protocol == "gemini" {
					path = "/v1beta/models/gpt-4o:generateContent"
					if stream {
						path = "/v1beta/models/gpt-4o:streamGenerateContent?alt=sse"
					}
					payload = map[string]any{"contents": []map[string]any{{"parts": []map[string]string{{"text": "hello"}}}}, "generationConfig": map[string]int{"maxOutputTokens": 16}}
				}
				done := make(chan apiResult, 1)
				go func() { done <- doRequest(env.base, "POST", path, env.apiKey, payload) }()
				select {
				case <-started:
				case <-time.After(5 * time.Second):
					t.Fatal("upstream did not receive the admitted request")
				}
				update(2, "20", "20")
				current := relay.ComputeUsageQuota(relay.NewPriceData("gpt-4o", "default"), usage)
				if current != 5000 {
					t.Fatalf("updated price=%d,want5000", current)
				}
				releaseOnce.Do(func() { close(release) })
				select {
				case result := <-done:
					if result.Status != 200 {
						t.Fatalf("relay: %d %s", result.Status, result.Body)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("in-flight request did not finish")
				}
				logs, count, err := model.ListLogs(env.rootID, 1, 10, "")
				if err != nil || count != 1 || logs[0].Quota != original {
					t.Fatalf("in-flight request was repriced: %+v count=%d %v", logs, count, err)
				}
				if result := doRequest(env.base, "POST", path, env.apiKey, payload); result.Status != 200 {
					t.Fatalf("next request: %d %s", result.Status, result.Body)
				}
				logs, count, err = model.ListLogs(env.rootID, 1, 10, "")
				if err != nil || count != 2 || logs[0].Quota != current || logs[1].Quota != original {
					t.Fatalf("next request did not use new price: %+v count=%d %v", logs, count, err)
				}
				user, err := model.GetUserByID(env.rootID)
				if err != nil || user.Quota != 100_000_000-original-current || user.UsedQuota != original+current {
					t.Fatalf("reservation/refund mismatch: %+v %v", user, err)
				}
				token, err := model.GetTokenByID(env.tokenID)
				if err != nil || token.UsedQuota != original+current {
					t.Fatal("token usage did not match captured bills")
				}
				var quote struct {
					Data struct {
						Model struct {
							Pricing struct {
								Input  float64 `json:"input_per_m"`
								Output float64 `json:"output_per_m"`
							} `json:"pricing"`
						} `json:"model"`
					} `json:"data"`
				}
				result := doRequest(env.base, "GET", "/api/plaza/models/gpt-4o", env.session, nil)
				if err := json.Unmarshal([]byte(result.Body), &quote); err != nil || quote.Data.Model.Pricing.Input != 20 || quote.Data.Model.Pricing.Output != 400 {
					t.Fatal("public quote did not retain current configuration")
				}
			})
		}
	}
}

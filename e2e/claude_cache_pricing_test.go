package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"starhub/common"
	"starhub/model"
	"starhub/relay"
	"starhub/relay/channel"
	"starhub/relay/dto"
)

func TestClaudeCachedRequestsKeepCompleteUsageAndAdmissionPrices(t *testing.T) {
	for _, mode := range []string{"tokens", "flat", "expression", "independent"} {
		for _, native := range []bool{false, true} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/native%t/stream%t", mode, native, stream), func(t *testing.T) {
					env := setupRelayApp(t)
					option := func(key, value string) {
						t.Helper()
						result := doRequest(env.base, "PUT", "/api/option", env.session, map[string]any{"key": key, "value": value})
						if result.Status != 200 {
							t.Fatalf("option %s: %d %s", key, result.Status, result.Body)
						}
					}
					option("model_ratio", `{"cached-model":2}`)
					option("completion_ratio", `{"cached-model":3}`)
					option("cache_ratio", "0.1")
					option("group_ratio", `{"default":0.8}`)
					option("model_time_pricing", `{"cached-model":{"timezone":"UTC","windows":[{"id":"all","label":"All week","days":[0,1,2,3,4,5,6],"start":"00:00","end":"24:00","multiplier":0.5}]}}`)
					option("model_price", "{}")
					option("model_price_expr", "{}")
					independent := func(multiplier int) {
						t.Helper()
						version, err := model.OptionVersion()
						if err != nil {
							t.Fatal(err)
						}
						rules := fmt.Sprintf(`{"cached-model":{"timezone":"UTC","windows":[{"id":"absolute","label":"Absolute","days":[0,1,2,3,4,5,6],"start":"00:00","end":"24:00","prices":{"input_per_m":"%d","output_per_m":"%d","cache_read_per_m":"%d","cache_write_per_m":"%d","audio_input_per_m":"%d","audio_output_per_m":"%d"}}]}}`, multiplier, multiplier*2, multiplier*3, multiplier*4, multiplier*5, multiplier*6)
						result := doRequest(env.base, "PUT", "/api/option/time-token-pricing", env.session, map[string]any{"expected_version": version, "rules": json.RawMessage(rules)})
						if result.Status != 200 {
							t.Fatalf("absolute window: %d %s", result.Status, result.Body)
						}
					}
					if mode == "independent" {
						independent(1)
					}
					if mode == "flat" {
						option("model_price", `{"cached-model":0.02}`)
					}
					if mode == "expression" {
						option("model_price_expr", `{"cached-model":"total_tokens * 0.000002"}`)
					}
					usage := &relay.Usage{PromptTokens: 160, CompletionTokens: 20, PromptTokensDetails: &dto.PromptTokensDetails{CachedTokens: 100, CacheWriteTokens: 50}}
					original := relay.ComputeUsageQuota(relay.NewPriceData("cached-model", "default"), usage)
					want := map[string]int64{"tokens": 52, "flat": 4000, "expression": 72, "independent": 110}[mode]
					if original != want {
						t.Fatalf("fixture fee%d want%d", original, want)
					}
					body := `{"id":"cached","model":"cached-model","content":[{"type":"text","text":"local"}],"stop_reason":"end_turn","usage":{"input_tokens":10,"cache_read_input_tokens":100,"cache_creation_input_tokens":50,"output_tokens":20}}`
					if stream {
						body = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"cached\",\"model\":\"cached-model\",\"usage\":{\"input_tokens\":10,\"cache_read_input_tokens\":100,\"cache_creation_input_tokens\":50,\"output_tokens\":0}}}\n\nevent: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"local\"}}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":3}}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":20}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
					}
					started, release := make(chan struct{}, 1), make(chan struct{})
					var once sync.Once
					var calls atomic.Int64
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						if r.URL.Path != "/v1/messages" || r.Header.Get("x-api-key") != "synthetic-cache-key" {
							t.Error("incorrect provider path or credential")
						}
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
						_, _ = io.WriteString(w, body)
					}))
					defer upstream.Close()
					defer once.Do(func() { close(release) })
					if err := model.InsertChannel(&model.Channel{Name: "Cached Claude", Type: channel.ChannelTypeClaude, Key: "synthetic-cache-key", BaseURL: upstream.URL, Models: "cached-model", Group: "default", Status: common.ChannelStatusEnabled}); err != nil {
						t.Fatal(err)
					}
					path := "/v1/chat/completions"
					if native {
						path = "/v1/messages"
					}
					payload := map[string]any{"model": "cached-model", "messages": []map[string]string{{"role": "user", "content": "local"}}, "max_tokens": 32, "stream": stream}
					done := make(chan apiResult, 1)
					go func() { done <- doRequest(env.base, "POST", path, env.apiKey, payload) }()
					select {
					case <-started:
					case <-time.After(5 * time.Second):
						t.Fatal("request was not admitted")
					}
					option("model_ratio", `{"cached-model":20}`)
					option("completion_ratio", `{"cached-model":10}`)
					option("cache_ratio", "0.9")
					option("group_ratio", `{"default":2}`)
					option("model_time_pricing", "{}")
					if mode == "independent" {
						independent(10)
					}
					if mode == "flat" {
						option("model_price", `{"cached-model":0.05}`)
					}
					if mode == "expression" {
						option("model_price_expr", `{"cached-model":"total_tokens * 0.00001"}`)
					}
					current := relay.ComputeUsageQuota(relay.NewPriceData("cached-model", "default"), usage)
					if current == original {
						t.Fatal("fixture edit did not change serving prices")
					}
					once.Do(func() { close(release) })
					var result apiResult
					select {
					case result = <-done:
					case <-time.After(5 * time.Second):
						t.Fatal("admitted request did not finish")
					}
					if result.Status != 200 {
						t.Fatalf("relay: %d %s", result.Status, result.Body)
					}
					if native {
						if result.Body != body {
							t.Fatal("native cached payload changed")
						}
					} else if !strings.Contains(result.Body, `"prompt_tokens":160`) || !strings.Contains(result.Body, `"cached_tokens":100`) || !strings.Contains(result.Body, `"cache_write_tokens":50`) {
						t.Fatalf("converted cached usage omitted: %s", result.Body)
					}
					logs, total, err := model.ListLogs(env.rootID, 1, 10, "")
					if err != nil || total != 1 || logs[0].PromptTokens != 160 || logs[0].CompletionTokens != 20 || logs[0].Quota != original {
						t.Fatalf("cached admission bill changed: %+v %d %v", logs, total, err)
					}
					owner, err := model.GetUserByID(env.rootID)
					if err != nil || owner.Quota != 100_000_000-original || owner.UsedQuota != original {
						t.Fatal("cached quota settlement duplicated or incomplete")
					}
					token, err := model.GetTokenByID(env.tokenID)
					if err != nil || token.UsedQuota != original {
						t.Fatal("token accounting differs from bill")
					}
					if next := doRequest(env.base, "POST", path, env.apiKey, payload); next.Status != 200 {
						t.Fatalf("next request: %d %s", next.Status, next.Body)
					}
					logs, total, err = model.ListLogs(env.rootID, 1, 10, "")
					if err != nil || total != 2 || logs[0].Quota != current || logs[1].Quota != original || calls.Load() != 2 {
						t.Fatal("new request did not use new prices or history changed")
					}
					owner, err = model.GetUserByID(env.rootID)
					token, tokenErr := model.GetTokenByID(env.tokenID)
					if err != nil || tokenErr != nil || owner.Quota != 100_000_000-original-current || owner.UsedQuota != original+current || token.UsedQuota != original+current {
						t.Fatal("subsequent cached settlement disagrees with immutable bills")
					}
				})
			}
		}
	}
}

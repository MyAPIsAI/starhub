package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
	"starhub/relay"
	"starhub/relay/channel"
	"starhub/relay/dto"
)

func publishCompatibleFixture(t *testing.T, ch *model.Channel) {
	t.Helper()
	ctx := context.Background()
	name := "gpt-4o"
	snapshot := model.ResourceSnapshot{ID: "provider-fixture", Sequence: 1, PublishedAt: "2026-11-19T00:00:00Z", Source: "local fixture",
		Channels:  []model.ResourceChannel{{ID: "provider", Name: "Provider", Enabled: true}},
		Instances: []model.ResourceInstance{{ID: "resource", ChannelID: "provider", CanonicalModel: name, UpstreamModel: name, Protocols: []string{"OpenAI"}, Capabilities: []string{"text", "reasoning", "function_call"}, ContextLength: 128000, Status: "available"}},
	}
	catalog, err := model.SaveResourceSnapshot(ctx, 0, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = model.SaveResourcePlacement(ctx, catalog.Revision, "resource", model.ResourcePlacement{ModelID: &name, GroupIDs: []string{"default"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	state, err := model.ReadResourcePublicationContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	expected := model.ResourcePublicationPrecondition{ResourceRevision: state.ResourceRevision, ChannelRevision: state.ChannelRevision, OptionVersion: state.OptionVersion}
	bindings := []model.ResourceChannelBinding{{SourceChannelID: "provider", GatewayChannelID: strconv.FormatInt(ch.ID, 10)}}
	plan, err := model.PreviewResourcePublication(ctx, expected, bindings)
	if err != nil || !plan.Ready {
		t.Fatalf("publication preview: %+v %v", plan, err)
	}
	if _, err = model.PublishResourceGateway(ctx, expected, bindings, plan.ReviewDigest, 1); err != nil {
		t.Fatal(err)
	}
}

func TestCompatibleProviderPublishedRelayAndBilling(t *testing.T) {
	for _, provider := range []struct {
		typeID int
		path   string
	}{
		{channel.ChannelTypeZhipu, "/api/paas/v4/chat/completions"},
		{channel.ChannelTypeMoonshot, "/v1/chat/completions"},
		{channel.ChannelTypeDeepSeek, "/chat/completions"},
	} {
		for _, inbound := range []dto.InboundFormat{dto.InboundOpenAI, dto.InboundClaude} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%d/%s/stream=%t", provider.typeID, inbound, stream), func(t *testing.T) {
					newTestDB(t)
					usage := `{"prompt_tokens":100,"completion_tokens":50,"completion_tokens_details":{"reasoning_tokens":30},"prompt_tokens_details":{"cached_tokens":60,"cache_write_tokens":10}}`
					if provider.typeID == channel.ChannelTypeDeepSeek {
						usage = `{"prompt_tokens":100,"completion_tokens":50,"prompt_cache_hit_tokens":60,"prompt_cache_miss_tokens":40,"completion_tokens_details":{"reasoning_tokens":30}}`
					}
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method != "POST" || r.URL.Path != provider.path || r.Header.Get("Authorization") != "Bearer fixture-upstream-key" {
							t.Errorf("unexpected upstream: %s %s", r.Method, r.URL.Path)
						}
						var request struct {
							Model         string           `json:"model"`
							Stream        bool             `json:"stream"`
							Thinking      json.RawMessage  `json:"thinking"`
							StreamOptions map[string]any   `json:"stream_options"`
							Messages      []map[string]any `json:"messages"`
						}
						if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Model != "gpt-4o" || request.Stream != stream {
							t.Errorf("request: %+v %v", request, err)
						}
						if inbound == dto.InboundOpenAI && (!strings.Contains(string(request.Thinking), "enabled") || request.Messages[0]["reasoning_content"] != "preserved fixture") {
							t.Error("native provider extensions were lost")
						}
						if stream && provider.typeID != channel.ChannelTypeZhipu && request.StreamOptions["include_usage"] != true {
							t.Error("stream usage not requested")
						}
						if stream && provider.typeID == channel.ChannelTypeZhipu && request.StreamOptions != nil {
							t.Error("foreign stream options sent to Zhipu")
						}
						if !stream {
							w.Header().Set("Content-Type", "application/json")
							fmt.Fprintf(w, `{"id":"provider-result","model":"gpt-4o","choices":[{"message":{"role":"assistant","content":"local result","reasoning_content":"fixture reasoning"},"finish_reason":"stop"}],"usage":%s}`, usage)
							return
						}
						w.Header().Set("Content-Type", "text/event-stream")
						fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"fixture reasoning\"}}],\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":1}}\n\n")
						if provider.typeID == channel.ChannelTypeMoonshot {
							fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"local result\"},\"finish_reason\":\"stop\",\"usage\":%s}]}\n\n", usage)
						} else {
							fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"local result\"},\"finish_reason\":\"stop\"}],\"usage\":%s}\n\n", usage)
						}
						fmt.Fprint(w, "data: [DONE]\n\n")
					}))
					defer upstream.Close()
					ch := &model.Channel{Name: "provider fixture", Type: provider.typeID, Key: "fixture-upstream-key", BaseURL: upstream.URL, Models: "gpt-4o", Group: "default", Status: common.ChannelStatusEnabled, Weight: 1}
					if err := model.InsertChannel(ch); err != nil {
						t.Fatal(err)
					}
					publishCompatibleFixture(t, ch)
					user := newUser(t, "provider-reader", common.RoleUser, 1000000)
					token := mustToken(t, user.ID, 1000000)
					handler, path := http.HandlerFunc(RelayChat), "/v1/chat/completions"
					if inbound == dto.InboundClaude {
						handler, path = RelayClaudeChat, "/v1/messages"
					}
					body := fmt.Sprintf(`{"model":"gpt-4o","messages":[{"role":"user","content":"Hello"}],"max_tokens":64,"stream":%t}`, stream)
					if inbound == dto.InboundOpenAI {
						body = fmt.Sprintf(`{"model":"gpt-4o","messages":[{"role":"assistant","content":"Earlier answer","reasoning_content":"preserved fixture"},{"role":"user","content":"Hello"}],"thinking":{"type":"enabled"},"max_tokens":64,"stream":%t}`, stream)
					}
					r := httptest.NewRequest("POST", path, strings.NewReader(body))
					r.Header.Set("Content-Type", "application/json")
					r.Header.Set("Authorization", "Bearer "+token.PlaintextKey())
					rec := httptest.NewRecorder()
					middleware.TokenAuth(middleware.Distribute(inbound)(handler)).ServeHTTP(rec, r)
					if rec.Code != 200 || !strings.Contains(rec.Body.String(), "local result") {
						t.Fatalf("relay: %d %s", rec.Code, rec.Body)
					}
					if inbound == dto.InboundOpenAI && !strings.Contains(rec.Body.String(), `"reasoning_content":"fixture reasoning"`) {
						t.Fatal("native response lost reasoning")
					}
					entries, total, err := model.ListLogs(user.ID, 1, 10, "gpt-4o")
					if err != nil || total != 1 || len(entries) != 1 {
						t.Fatalf("logs: %+v %v", entries, err)
					}
					entry := entries[0]
					if entry.ChannelID != ch.ID || entry.PromptTokens != 100 || entry.CompletionTokens != 50 || entry.Status != 1 {
						t.Fatalf("usage not settled once: %+v", entry)
					}
					var expected relay.Usage
					if err := json.Unmarshal([]byte(usage), &expected); err != nil {
						t.Fatal(err)
					}
					if provider.typeID == channel.ChannelTypeDeepSeek {
						expected.PromptTokensDetails = &relay.PromptTokensDetails{CachedTokens: 60}
					}
					if want := strconv.FormatInt(relay.ComputeUsageNano(relay.NewPriceData("gpt-4o", "default"), &expected), 10); entry.ExactFeeNanoUSD != want {
						t.Fatalf("cache pricing: %s, want %s", entry.ExactFeeNanoUSD, want)
					}
					if inbound == dto.InboundOpenAI && !stream {
						before, _ := model.GetUserByID(user.ID)
						deny := func(body string) {
							t.Helper()
							r := httptest.NewRequest("POST", path, strings.NewReader(body))
							r.Header.Set("Content-Type", "application/json")
							r.Header.Set("Authorization", "Bearer "+token.PlaintextKey())
							rec := httptest.NewRecorder()
							middleware.TokenAuth(middleware.Distribute(inbound)(handler)).ServeHTTP(rec, r)
							if rec.Code < 400 {
								t.Fatalf("unclassified reasoning accepted: %s", rec.Body)
							}
						}
						deny(strings.Replace(body, `"reasoning_content":"preserved fixture"`, `"reasoning_content":{"image_url":"hidden"}`, 1))
						deny(strings.Replace(body, `"role":"assistant"`, `"role":"user"`, 1))
						if err := model.DB.Model(&model.ResourceGatewayRoute{}).Where("channel_id = ?", ch.ID).Update("capabilities", `["text"]`).Error; err != nil {
							t.Fatal(err)
						}
						deny(body)
						after, _ := model.GetUserByID(user.ID)
						if after.Quota != before.Quota || after.QuotaRemainderNano != before.QuotaRemainderNano {
							t.Fatal("rejected reasoning moved quota")
						}
						if _, total, err := model.ListLogs(user.ID, 1, 10, "gpt-4o"); err != nil || total != 1 {
							t.Fatal("rejected reasoning created consumption", err)
						}
					}
				})
			}
		}
	}
}

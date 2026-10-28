package controller

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
	"starhub/relay"
	"starhub/relay/channel"
	"starhub/relay/dto"
)

func TestRelayAutoModelWithoutCandidatesDoesNotCharge(t *testing.T) {
	newTestDB(t)
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"unexpected","object":"chat.completion","model":"myapis-auto","choices":[{"index":0,"message":{"role":"assistant","content":"unexpected"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	defer upstream.Close()
	if err := model.InsertChannel(&model.Channel{Name: "virtual-only", Type: channel.ChannelTypeOpenAI, Key: "review-key", BaseURL: upstream.URL, Models: "myapis-auto,veyra-auto", Group: "default", Status: common.ChannelStatusEnabled, Weight: 1}); err != nil {
		t.Fatal(err)
	}
	user := newUser(t, "auto-unavailable", common.RoleUser, 1000000)
	token := mustToken(t, user.ID, 1000000)
	handler := middleware.TokenAuth(middleware.Distribute(dto.InboundOpenAI)(http.HandlerFunc(RelayChat)))
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"myapis-auto","messages":[{"role":"user","content":"Hello"}],"max_tokens":64}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+token.Key)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, r)
	if rec.Code != http.StatusServiceUnavailable || calls.Load() != 0 {
		t.Fatalf("status = %d, upstream calls = %d; body %s", rec.Code, calls.Load(), rec.Body)
	}
	afterUser, err := model.GetUserByID(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	afterToken, err := model.GetTokenByID(token.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterUser.Quota != user.Quota || afterUser.UsedQuota != user.UsedQuota || afterToken.RemainQuota != token.RemainQuota || afterToken.UsedQuota != token.UsedQuota {
		t.Fatal("candidate unavailability changed user or token quota")
	}
	if count, err := model.CountLogs(user.ID); err != nil || count != 0 {
		t.Fatalf("pre-channel failure created a log: count=%d err=%v", count, err)
	}
}

func TestRelayAutoModelReachesUpstream(t *testing.T) {
	for _, tc := range []struct {
		name      string
		inbound   dto.InboundFormat
		kind      int
		requested string
	}{
		{"openai passthrough", dto.InboundOpenAI, channel.ChannelTypeOpenAI, "myapis-auto"},
		{"claude passthrough", dto.InboundClaude, channel.ChannelTypeClaude, "myapis-auto"},
		{"openai to claude", dto.InboundOpenAI, channel.ChannelTypeClaude, "myapis-auto"},
		{"claude to openai", dto.InboundClaude, channel.ChannelTypeOpenAI, "myapis-auto"},
		{"explicit model", dto.InboundOpenAI, channel.ChannelTypeOpenAI, "gpt-4o"},
		{"legacy openai passthrough", dto.InboundOpenAI, channel.ChannelTypeOpenAI, "veyra-auto"},
		{"legacy claude passthrough", dto.InboundClaude, channel.ChannelTypeClaude, "veyra-auto"},
		{"legacy openai to claude", dto.InboundOpenAI, channel.ChannelTypeClaude, "veyra-auto"},
		{"legacy claude to openai", dto.InboundClaude, channel.ChannelTypeOpenAI, "veyra-auto"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			newTestDB(t)
			captured := make(chan []byte, common.RetryTimes+1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, "read failed", http.StatusBadRequest)
					return
				}
				captured <- body
				w.Header().Set("Content-Type", "application/json")
				if tc.kind == channel.ChannelTypeClaude {
					_, _ = io.WriteString(w, `{"id":"msg_test","type":"message","role":"assistant","model":"gpt-4o","content":[{"type":"text","text":"Hello"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
				} else {
					_, _ = io.WriteString(w, `{"id":"chatcmpl-test","object":"chat.completion","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"Hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
				}
			}))
			defer upstream.Close()
			if err := model.InsertChannel(&model.Channel{Name: "auto", Type: tc.kind, Key: "review-key", BaseURL: upstream.URL, Models: "gpt-4o", Group: "default", Status: common.ChannelStatusEnabled, Weight: 1}); err != nil {
				t.Fatal(err)
			}
			user := newUser(t, "auto-review", common.RoleUser, 1000000)
			token := mustToken(t, user.ID, 1000000)
			path, relayHandler := "/v1/chat/completions", http.HandlerFunc(RelayChat)
			if tc.inbound == dto.InboundClaude {
				path, relayHandler = "/v1/messages", RelayClaudeChat
			}
			handler := middleware.TokenAuth(middleware.Distribute(tc.inbound)(relayHandler))
			body := `{"model":"` + tc.requested + `","messages":[{"role":"user","content":"Hello"}],"max_tokens":64,"stream":false}`
			r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Authorization", "Bearer "+token.Key)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, r)
			if rec.Code != http.StatusOK {
				t.Fatalf("relay failed: %d %s", rec.Code, rec.Body)
			}
			entries, total, err := model.ListLogs(user.ID, 1, 10, "gpt-4o")
			if err != nil || total != 1 || len(entries) != 1 {
				t.Fatalf("success log not persisted: total %d, entries %d, error %v", total, len(entries), err)
			}
			expectedProtocol := "OpenAI"
			if tc.inbound == dto.InboundClaude {
				expectedProtocol = "Anthropic"
			}
			if entries[0].Protocol != expectedProtocol {
				t.Fatalf("protocol attribution lost: %+v", entries[0])
			}
			if entries[0].RequestedModel != tc.requested || entries[0].Model != "gpt-4o" || entries[0].Status != 1 {
				t.Fatalf("requested/actual model attribution lost: %+v", entries[0])
			}
			var page struct {
				Items []struct {
					RequestedModel string `json:"requested_model"`
					Model          string `json:"model"`
				} `json:"items"`
			}
			decodeData(t, call(ListLogs, testRequest{target: "/api/log", userID: user.ID, role: common.RoleUser}), &page)
			if len(page.Items) != 1 || page.Items[0].RequestedModel != tc.requested || page.Items[0].Model != "gpt-4o" {
				t.Fatalf("log API lost model attribution: %+v", page)
			}
			select {
			case sent := <-captured:
				var payload struct {
					Model string `json:"model"`
				}
				if err := json.Unmarshal(sent, &payload); err != nil {
					t.Fatal(err)
				}
				if payload.Model != "gpt-4o" {
					t.Fatalf("upstream model = %q, want gpt-4o; body %s", payload.Model, sent)
				}
			default:
				t.Fatal("relay returned without reaching the local upstream")
			}
		})
	}
}

func TestPassthroughModelAttribution(t *testing.T) {
	for _, tc := range []struct {
		name       string
		path       string
		handler    http.HandlerFunc
		actual     string
		fields     string
		response   string
		completion int64
	}{
		{"embeddings", "/v1/embeddings", RelayEmbeddings, "text-embedding-3-small", `"input":"Hello"`, `{"object":"list","data":[{"object":"embedding","embedding":[0.1],"index":0}],"usage":{"prompt_tokens":12,"total_tokens":12}}`, 0},
		{"rerank", "/v1/rerank", RelayRerank, "rerank-review", `"query":"Hello","documents":["Hello world"]`, `{"results":[{"index":0,"relevance_score":0.9}],"usage":{"input_tokens":12,"output_tokens":2,"total_tokens":14}}`, 2},
	} {
		for _, requested := range []string{"myapis-auto", tc.actual} {
			t.Run(tc.name+"/"+requested, func(t *testing.T) {
				newTestDB(t)
				captured := make(chan []byte, common.RetryTimes+1)
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(r.Body)
					if err != nil || r.URL.Path != tc.path || r.Method != http.MethodPost {
						http.Error(w, "invalid upstream request", http.StatusBadRequest)
						return
					}
					captured <- body
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, tc.response)
				}))
				defer upstream.Close()
				ch := &model.Channel{Name: "passthrough-review", Type: channel.ChannelTypeOpenAI, Key: "review-key", BaseURL: upstream.URL, Models: tc.actual, Group: "default", Status: common.ChannelStatusEnabled, Weight: 1}
				if err := model.InsertChannel(ch); err != nil {
					t.Fatal(err)
				}
				if tc.path == "/v1/embeddings" || tc.path == "/v1/rerank" {
					tag := "embedding"
					if tc.path == "/v1/rerank" {
						tag = "rerank"
					}
					if err := model.UpsertModelMeta(&model.ModelMeta{Model: tc.actual, Tags: tag}); err != nil {
						t.Fatal(err)
					}
				}
				user := newUser(t, "passthrough-review", common.RoleUser, 1000000)
				token := mustToken(t, user.ID, 1000000)
				handler := middleware.TokenAuth(middleware.Distribute(dto.InboundOpenAI)(tc.handler))
				body := `{"model":"` + requested + `",` + tc.fields + `}`
				r := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(body))
				r.Header.Set("Content-Type", "application/json")
				r.Header.Set("Authorization", "Bearer "+token.Key)
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, r)
				if rec.Code != http.StatusOK || rec.Body.String() != tc.response {
					t.Fatalf("passthrough response = %d %s", rec.Code, rec.Body)
				}
				select {
				case sent := <-captured:
					var payload struct {
						Model string `json:"model"`
					}
					if err := json.Unmarshal(sent, &payload); err != nil || payload.Model != tc.actual {
						t.Fatalf("upstream model = %q, err = %v, want %q", payload.Model, err, tc.actual)
					}
				default:
					t.Fatal("request did not reach upstream")
				}
				entries, total, err := model.ListLogs(user.ID, 1, 10, tc.actual)
				if err != nil || total != 1 || len(entries) != 1 {
					t.Fatalf("success log missing: total=%d entries=%d err=%v", total, len(entries), err)
				}
				entry := entries[0]
				if entry.RequestedModel != requested || entry.Model != tc.actual || entry.ChannelID != ch.ID || entry.Status != 1 {
					t.Fatalf("model attribution lost: %+v", entry)
				}
				charge := relay.ComputeQuota(tc.actual, "default", 12, tc.completion)
				if entry.Quota != charge || entry.PromptTokens != 12 || entry.CompletionTokens != tc.completion {
					t.Fatalf("usage changed: %+v, want charge=%d tokens=12/%d", entry, charge, tc.completion)
				}
				afterUser, err := model.GetUserByID(user.ID)
				if err != nil {
					t.Fatal(err)
				}
				afterToken, err := model.GetTokenByID(token.ID)
				if err != nil {
					t.Fatal(err)
				}
				if afterUser.Quota != user.Quota-charge || afterUser.UsedQuota != user.UsedQuota+charge || afterToken.RemainQuota != token.RemainQuota-charge || afterToken.UsedQuota != token.UsedQuota+charge {
					t.Fatal("settlement did not charge the actual model exactly once")
				}
				var page struct {
					Items []model.Log `json:"items"`
				}
				decodeData(t, call(ListLogs, testRequest{target: "/api/log", userID: user.ID, role: common.RoleUser}), &page)
				if len(page.Items) != 1 || page.Items[0].RequestedModel != requested || page.Items[0].Model != tc.actual {
					t.Fatalf("log API lost attribution: %+v", page)
				}
			})
		}
	}
}

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
	"starhub/relay/channel"
	"starhub/relay/dto"
	"starhub/setting"
)

func TestSmartToolClientsDoNotNeedCustomHeaders(t *testing.T) {
	for _, inbound := range []dto.InboundFormat{dto.InboundOpenAI, dto.InboundClaude} {
		for _, alias := range []string{"myapis-auto", "veyra-auto"} {
			t.Run(string(inbound)+"/"+alias, func(t *testing.T) {
				newTestDB(t)
				var calls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					n := calls.Add(1)
					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Error(err)
					}
					var payload struct {
						Model    string            `json:"model"`
						Messages []json.RawMessage `json:"messages"`
					}
					if err := json.Unmarshal(body, &payload); err != nil || payload.Model != "gpt-4o" {
						t.Errorf("tool chain rerouted: %s %v", body, err)
					}
					if n == 2 && (!strings.Contains(string(body), "call_1") || !strings.Contains(string(body), "tool output")) {
						t.Errorf("tool result lost: %s", body)
					}
					w.Header().Set("Content-Type", "application/json")
					if n == 1 {
						_, _ = w.Write([]byte(`{"id":"review","object":"chat.completion","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":12,"completion_tokens":4,"total_tokens":16}}`))
					} else {
						_, _ = w.Write([]byte(`{"id":"review","object":"chat.completion","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],"usage":{"prompt_tokens":20,"completion_tokens":4,"total_tokens":24}}`))
					}
				}))
				defer upstream.Close()
				ch := &model.Channel{Name: "tool-client", Type: channel.ChannelTypeOpenAI, Models: "gpt-4o,gpt-4o-mini", Group: "default", Status: common.ChannelStatusEnabled, Key: "local-review", BaseURL: upstream.URL, Weight: 1}
				if err := model.InsertChannel(ch); err != nil {
					t.Fatal(err)
				}
				for _, name := range []string{"gpt-4o", "gpt-4o-mini"} {
					if err := model.UpsertModelMeta(&model.ModelMeta{Model: name, Tags: "text,function_call", ContextLength: 4096, MaxOutput: 1024}); err != nil {
						t.Fatal(err)
					}
				}
				if err := setting.UpdateRatio(setting.OptionKeyModelRatio, `{"gpt-4o":0.1,"gpt-4o-mini":10}`); err != nil {
					t.Fatal(err)
				}
				u := newUser(t, "tool-client", common.RoleUser, 1000000)
				tk := mustToken(t, u.ID, 1000000)
				first := `{"model":"` + alias + `","messages":[{"role":"user","content":"review tools"}],"max_tokens":8,"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object"}}}]}`
				next := `{"model":"` + alias + `","messages":[{"role":"user","content":"review tools"},{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call_1","content":"tool output"}],"max_tokens":8}`
				path, handler := "/v1/chat/completions", http.HandlerFunc(RelayChat)
				if inbound == dto.InboundClaude {
					path, handler = "/v1/messages", RelayClaudeChat
					first = `{"model":"` + alias + `","messages":[{"role":"user","content":"review tools"}],"max_tokens":8,"tools":[{"name":"lookup","input_schema":{"type":"object"}}]}`
					next = `{"model":"` + alias + `","messages":[{"role":"user","content":"review tools"},{"role":"assistant","content":[{"type":"tool_use","id":"call_1","name":"lookup","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":"tool output"}]}],"max_tokens":8}`
				}
				for turn, body := range []string{first, next} {
					r := httptest.NewRequest("POST", path, strings.NewReader(body))
					r.Header.Set("Content-Type", "application/json")
					if inbound == dto.InboundClaude {
						r.Header.Set("x-api-key", tk.Key)
						r.Header.Set("anthropic-version", "2023-06-01")
					} else {
						r.Header.Set("Authorization", "Bearer "+tk.Key)
					}
					rec := httptest.NewRecorder()
					middleware.TokenAuth(middleware.Distribute(inbound)(handler)).ServeHTTP(rec, r)
					if rec.Code != 200 || (turn == 0 && !strings.Contains(rec.Body.String(), "call_1")) {
						t.Fatalf("client tool request failed: turn=%d status=%d %s", turn, rec.Code, rec.Body)
					}
					if err := setting.UpdateRatio(setting.OptionKeyModelRatio, `{"gpt-4o":10,"gpt-4o-mini":0.1}`); err != nil {
						t.Fatal(err)
					}
				}
				logs, total, err := model.ListLogs(u.ID, 1, 10, "")
				if err != nil {
					t.Fatal(err)
				}
				var spent int64
				for _, entry := range logs {
					if entry.Model != "gpt-4o" || entry.RequestedModel != alias || entry.Status != 1 {
						t.Fatalf("incorrect tool log: %+v", entry)
					}
					spent += entry.Quota
				}
				after, err := model.GetUserByID(u.ID)
				if err != nil {
					t.Fatal(err)
				}
				if total != 2 || calls.Load() != 2 || spent <= 0 || after.Quota != u.Quota-spent {
					t.Fatalf("incorrect tool chain settlement: total=%d calls=%d spent=%d quota=%d", total, calls.Load(), spent, after.Quota)
				}
			})
		}
	}
}

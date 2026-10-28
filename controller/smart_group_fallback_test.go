package controller

import (
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
	"starhub/setting"
)

func TestSmartChatUsesAvailableAutoGroupForPrecharge(t *testing.T) {
	for _, requested := range []string{"myapis-auto", "veyra-auto", "gpt-4o"} {
		for _, inbound := range []dto.InboundFormat{dto.InboundOpenAI, dto.InboundClaude} {
			t.Run(requested+"/"+string(inbound), func(t *testing.T) {
				newTestDB(t)
				if err := setting.UpdateRatio(setting.OptionKeyGroupRatio, `{"empty":100,"default":2}`); err != nil {
					t.Fatal(err)
				}
				var calls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					body, err := io.ReadAll(r.Body)
					if err != nil || !strings.Contains(string(body), `"model":"gpt-4o"`) {
						t.Errorf("invalid routed body: %s %v", body, err)
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"id":"review","object":"chat.completion","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":12,"completion_tokens":4,"total_tokens":16}}`))
				}))
				defer upstream.Close()
				ch := &model.Channel{Name: "fallback", Type: channel.ChannelTypeOpenAI, Models: "gpt-4o", Group: "default", Status: common.ChannelStatusEnabled, Key: "local-review", BaseURL: upstream.URL, Weight: 1}
				if err := model.InsertChannel(ch); err != nil {
					t.Fatal(err)
				}
				body := `{"model":"` + requested + `","messages":[{"role":"user","content":"hello"}],"max_tokens":8}`
				budget := relay.EstimatePreConsumeQuota("gpt-4o", "default", []byte(body)) + 1
				if relay.EstimatePreConsumeQuota("gpt-4o", "empty", []byte(body)) <= budget {
					t.Fatal("fixture does not distinguish the unavailable group's reserve price")
				}
				u := newUser(t, "smart-fallback", common.RoleUser, budget)
				tk := mustToken(t, u.ID, budget)
				if err := model.DB.Model(&model.Token{}).Where("id = ?", tk.ID).Updates(map[string]any{"group": common.AutoGroup, "auto_groups": "empty,default", "model_limits": "gpt-4o"}).Error; err != nil {
					t.Fatal(err)
				}
				path, handler := "/v1/chat/completions", http.HandlerFunc(RelayChat)
				if inbound == dto.InboundClaude {
					path, handler = "/v1/messages", RelayClaudeChat
				}
				r := httptest.NewRequest("POST", path, strings.NewReader(body))
				r.Header.Set("Content-Type", "application/json")
				r.Header.Set("Authorization", "Bearer "+tk.Key)
				rec := httptest.NewRecorder()
				middleware.TokenAuth(middleware.Distribute(inbound)(handler)).ServeHTTP(rec, r)
				if rec.Code != 200 || calls.Load() != 1 {
					t.Fatalf("fallback failed: %d %s calls=%d", rec.Code, rec.Body, calls.Load())
				}
				logs, total, err := model.ListLogs(u.ID, 1, 10, "")
				if err != nil {
					t.Fatal(err)
				}
				actual := relay.ComputeQuota("gpt-4o", "default", 12, 4)
				after, err := model.GetUserByID(u.ID)
				if err != nil {
					t.Fatal(err)
				}
				keyAfter, err := model.GetTokenByID(tk.ID)
				if err != nil {
					t.Fatal(err)
				}
				if total != 1 || logs[0].RequestedModel != requested || logs[0].Model != "gpt-4o" || logs[0].ChannelID != ch.ID || logs[0].Quota != actual || after.Quota != u.Quota-actual || keyAfter.RemainQuota != tk.RemainQuota-actual {
					t.Fatalf("incorrect fallback settlement: logs=%+v quota=%d expected=%d", logs, after.Quota, actual)
				}
			})
		}
	}
}

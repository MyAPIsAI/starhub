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
	"starhub/relay/channel"
	"starhub/relay/dto"
)

func TestSmartChainAdmissionAfterValidationAndQuota(t *testing.T) {
	for _, tc := range []struct {
		name      string
		quota     int64
		maxTokens int
		conflict  bool
		want      int
		pins      int64
	}{
		{"insufficient quota", 0, 8, false, 403, 0},
		{"invalid request", 1000000, -1, false, 400, 0},
		{"concurrent conflict", 1000000, 8, true, 409, 1},
		{"admitted request", 1000000, 8, false, 200, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			newTestDB(t)
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var count int64
				if err := model.DB.Model(&model.RoutingChain{}).Count(&count).Error; err != nil || count != 1 {
					t.Errorf("upstream reached without durable pin: count=%d err=%v", count, err)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"chatcmpl-test","object":"chat.completion","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"Hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
			}))
			defer upstream.Close()
			if err := model.InsertChannel(&model.Channel{Name: "chain", Type: channel.ChannelTypeOpenAI, Key: "test", BaseURL: upstream.URL, Models: "gpt-4o", Group: "default", Status: common.ChannelStatusEnabled, Weight: 1}); err != nil {
				t.Fatal(err)
			}
			if err := model.UpsertModelMeta(&model.ModelMeta{Model: "gpt-4o", Tags: "text,function_call"}); err != nil {
				t.Fatal(err)
			}
			user := newUser(t, "chain-admission", common.RoleUser, tc.quota)
			token := mustToken(t, user.ID, 1000000)
			handler := middleware.TokenAuth(middleware.Distribute(dto.InboundOpenAI)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.conflict {
					if _, err := model.PinRoutingChain(token.ID, "default", "admission-test", "different-model"); err != nil {
						t.Fatal(err)
					}
				}
				RelayChat(w, r)
			})))
			body := `{"model":"myapis-auto","messages":[{"role":"user","content":"Hello"}],"max_tokens":8}`
			if tc.maxTokens < 0 {
				body = strings.Replace(body, `"max_tokens":8`, `"max_tokens":-1`, 1)
			}
			r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Authorization", "Bearer "+token.Key)
			r.Header.Set("X-MyAPIs-Chain-Id", "admission-test")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, r)
			if rec.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%s", rec.Code, tc.want, rec.Body)
			}
			var count int64
			if err := model.DB.Model(&model.RoutingChain{}).Count(&count).Error; err != nil || count != tc.pins {
				t.Fatalf("pins=%d want=%d err=%v", count, tc.pins, err)
			}
			if tc.want != 200 {
				afterUser, err := model.GetUserByID(user.ID)
				if err != nil {
					t.Fatal(err)
				}
				afterToken, err := model.GetTokenByID(token.ID)
				if err != nil {
					t.Fatal(err)
				}
				if calls.Load() != 0 || afterUser.Quota != user.Quota || afterToken.RemainQuota != token.RemainQuota {
					t.Fatalf("rejection sent upstream or changed quota: calls=%d user=%d token=%d", calls.Load(), afterUser.Quota, afterToken.RemainQuota)
				}
			}
		})
	}
}

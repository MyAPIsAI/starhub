package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"gorm.io/gorm"
	"starhub/common"
	"starhub/middleware"
	"starhub/model"
	"starhub/relay/channel"
	"starhub/relay/dto"
	"starhub/service"
	"starhub/setting"
)

func TestSyncRelaySettlementRecoversWithoutCallingUpstream(t *testing.T) {
	for _, tc := range []struct {
		name, path, body, response string
		handler                    http.HandlerFunc
	}{
		{"responses", "/v1/responses", `{"model":"gpt-4o","input":"Hello","max_output_tokens":8}`, `{"id":"resp_test","status":"completed","output":[],"usage":{"input_tokens":10,"output_tokens":2}}`, RelayResponses},
		{"chat", "/v1/chat/completions", `{"model":"gpt-4o","messages":[{"role":"user","content":"Hello"}],"max_tokens":8}`, `{"id":"chatcmpl-test","object":"chat.completion","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"Hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`, RelayChat},
		{"embeddings", "/v1/embeddings", `{"model":"gpt-4o","input":"Hello"}`, `{"object":"list","data":[],"usage":{"prompt_tokens":12,"total_tokens":12}}`, RelayEmbeddings},
		{"image", "/v1/images/generations", `{"model":"gpt-4o","prompt":"a cat","n":1}`, `{"data":[{"b64_json":"QUJD"}]}`, RelayImageGenerations},
	} {
		t.Run(tc.name, func(t *testing.T) {
			newTestDB(t)
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.response))
			}))
			defer upstream.Close()
			ch := &model.Channel{Name: "recovery", Type: channel.ChannelTypeOpenAI, Key: "test", BaseURL: upstream.URL, Models: "gpt-4o", Group: "default", Status: common.ChannelStatusEnabled, Weight: 1}
			if err := model.InsertChannel(ch); err != nil {
				t.Fatal(err)
			}
			if err := setting.UpdatePricing(setting.OptionKeyModelPrice, `{"gpt-4o":0.001}`); err != nil {
				t.Fatal(err)
			}
			user := newUser(t, "sync-recovery", common.RoleUser, 1000000)
			token := mustToken(t, user.ID, 1000000)
			callback := "test:sync_log_failure"
			if err := model.DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "Log" {
					tx.AddError(errors.New("injected log failure"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			defer model.DB.Callback().Create().Remove(callback)
			handler := middleware.TokenAuth(middleware.Distribute(dto.InboundOpenAI)(tc.handler))
			r := httptest.NewRequest("POST", tc.path, strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Authorization", "Bearer "+token.Key)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, r)
			if rec.Code != 200 {
				t.Fatalf("response corrupted by settlement failure: %d %s", rec.Code, rec.Body)
			}
			pending, err := model.ListPendingQuotaSettlements(200)
			if err != nil || len(pending) != 1 || pending[0].Actual <= 0 {
				t.Fatalf("frozen bill missing: %+v %v", pending, err)
			}
			actual := pending[0].Actual
			before, _ := model.GetUserByID(user.ID)
			if before.Quota >= user.Quota || before.UsedQuota != 0 {
				t.Fatalf("reservation lost or partial charge: %+v", before)
			}
			if err := model.DB.Callback().Create().Remove(callback); err != nil {
				t.Fatal(err)
			}
			upstream.Close()
			if err := setting.UpdatePricing(setting.OptionKeyModelPrice, `{"gpt-4o":100}`); err != nil {
				t.Fatal(err)
			}
			service.RetryPendingQuotaSettlementsOnce(context.Background())
			service.RetryPendingQuotaSettlementsOnce(context.Background())
			after, _ := model.GetUserByID(user.ID)
			afterToken, _ := model.GetTokenByID(token.ID)
			afterChannel, _ := model.GetChannelByID(ch.ID)
			if after.Quota != user.Quota-actual || after.UsedQuota != actual || afterToken.RemainQuota != token.RemainQuota-actual || afterToken.UsedQuota != actual || afterChannel.UsedQuota != actual || calls.Load() != 1 {
				t.Fatalf("recovery balance/call mismatch: user=%+v token=%+v channel=%+v calls=%d", after, afterToken, afterChannel, calls.Load())
			}
			logs, total, err := model.ListLogs(user.ID, 1, 10, "")
			if err != nil || total != 1 || logs[0].Quota != actual || logs[0].Status != 1 || logs[0].Model != "gpt-4o" || logs[0].RequestedModel != "gpt-4o" || logs[0].Protocol != "OpenAI" {
				t.Fatalf("recovery log mismatch: %+v %v", logs, err)
			}
		})
	}
}

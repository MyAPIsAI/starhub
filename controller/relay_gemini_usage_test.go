package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
	"starhub/relay"
	"starhub/relay/channel"
	"starhub/relay/dto"
)

func TestGeminiCompatibleRelayBillsFinalNativeUsage(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			newTestDB(t)
			body := `{"candidates":[{"content":{"role":"model","parts":[{"text":"Hello"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":20,"candidatesTokenCount":7,"toolUsePromptTokenCount":3,"thoughtsTokenCount":5,"cachedContentTokenCount":8,"totalTokenCount":35}}`
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("x-goog-api-key") != "supplier-key" || r.Header.Get("Authorization") != "" {
					t.Errorf("incorrect supplier authentication: %v", r.Header)
				}
				if stream {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"Hi\"}]}}],\"usageMetadata\":{\"promptTokenCount\":10,\"candidatesTokenCount\":2}}\n\n")
					_, _ = io.WriteString(w, "data: "+body+"\n\n")
				} else {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, body)
				}
			}))
			defer upstream.Close()
			ch := &model.Channel{Name: "gemini-usage", Type: channel.ChannelTypeGemini, Key: "supplier-key", BaseURL: upstream.URL, Models: "gemini-test", Group: "default", Status: common.ChannelStatusEnabled, Weight: 1}
			if err := model.InsertChannel(ch); err != nil {
				t.Fatal(err)
			}
			user := newUser(t, "gemini-usage", common.RoleUser, 1000000)
			token := mustToken(t, user.ID, 1000000)
			handler := middleware.TokenAuth(middleware.Distribute(dto.InboundOpenAI)(http.HandlerFunc(RelayChat)))
			r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(fmt.Sprintf(`{"model":"gemini-test","messages":[{"role":"user","content":"Hi"}],"max_tokens":64,"stream":%t}`, stream)))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Authorization", "Bearer "+token.Key)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, r)
			logs, count, err := model.ListLogs(user.ID, 1, 10, "")
			if rec.Code != 200 || err != nil || count != 1 || logs[0].PromptTokens != 23 || logs[0].CompletionTokens != 12 {
				t.Fatalf("final Gemini usage lost or accumulated: %d %s %+v %v", rec.Code, rec.Body, logs, err)
			}
			usage, _ := dto.ParseGeminiUsage([]byte(body))
			want := relay.ComputeUsageQuota(relay.NewPriceDataWithUserGroup("gemini-test", "default", "default"), usage)
			after, _ := model.GetUserByID(user.ID)
			if logs[0].Quota != want || after.Quota != user.Quota-want || after.UsedQuota != want {
				t.Fatalf("Gemini final bill mismatch: log=%+v user=%+v want=%d", logs[0], after, want)
			}
		})
	}
}

package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
	"starhub/relay/channel"
	"starhub/relay/dto"
)

func TestFailedRelayModelAttribution(t *testing.T) {
	for _, tc := range []struct {
		name      string
		requested string
		status    int
		channels  int
		inbound   dto.InboundFormat
	}{
		{"routed retry exhaustion", "myapis-auto", http.StatusInternalServerError, 2, dto.InboundOpenAI},
		{"routed client error", "myapis-auto", http.StatusBadRequest, 1, dto.InboundOpenAI},
		{"explicit client error", "gpt-4o", http.StatusBadRequest, 1, dto.InboundOpenAI},
		{"claude routed exhaustion", "myapis-auto", http.StatusInternalServerError, 2, dto.InboundClaude},
	} {
		t.Run(tc.name, func(t *testing.T) {
			newTestDB(t)
			var calls atomic.Int32
			captured := make(chan []byte, common.RetryTimes+1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				body, _ := io.ReadAll(r.Body)
				captured <- body
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, `{"type":"error","error":{"type":"api_error","message":"review failure"}}`)
			}))
			defer upstream.Close()
			kind, path, relayHandler := channel.ChannelTypeOpenAI, "/v1/chat/completions", http.HandlerFunc(RelayChat)
			if tc.inbound == dto.InboundClaude {
				kind, path, relayHandler = channel.ChannelTypeClaude, "/v1/messages", RelayClaudeChat
			}
			var channels []*model.Channel
			var trail []string
			for i := range tc.channels {
				ch := &model.Channel{Name: fmt.Sprintf("failed-review-%d", i), Type: kind, Key: "review-key", BaseURL: upstream.URL, Models: "gpt-4o", Group: "default", Status: common.ChannelStatusEnabled, Priority: int64(tc.channels - i), Weight: 1}
				if err := model.InsertChannel(ch); err != nil {
					t.Fatal(err)
				}
				channels = append(channels, ch)
				trail = append(trail, strconv.FormatInt(ch.ID, 10))
			}
			user := newUser(t, "failed-review", common.RoleUser, 1000000)
			token := mustToken(t, user.ID, 1000000)
			handler := middleware.TokenAuth(middleware.Distribute(tc.inbound)(relayHandler))
			body := fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"Hello"}],"max_tokens":64}`, tc.requested)
			r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Authorization", "Bearer "+token.Key)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, r)
			wantStatus := http.StatusBadGateway
			if tc.status == http.StatusBadRequest {
				wantStatus = tc.status
			}
			if rec.Code != wantStatus || calls.Load() != int32(tc.channels) {
				t.Fatalf("status=%d calls=%d, want %d/%d; body %s", rec.Code, calls.Load(), wantStatus, tc.channels, rec.Body)
			}
			for range tc.channels {
				var payload struct {
					Model string `json:"model"`
				}
				if err := json.Unmarshal(<-captured, &payload); err != nil || payload.Model != "gpt-4o" {
					t.Fatalf("upstream model=%q err=%v", payload.Model, err)
				}
			}
			entries, total, err := model.ListLogs(user.ID, 1, 10, "gpt-4o")
			if err != nil || total != 1 || len(entries) != 1 {
				t.Fatalf("failed log missing: total=%d entries=%d err=%v", total, len(entries), err)
			}
			entry := entries[0]
			expectedProtocol := "OpenAI"
			if tc.inbound == dto.InboundClaude {
				expectedProtocol = "Anthropic"
			}
			if entry.Protocol != expectedProtocol {
				t.Fatalf("failed protocol attribution lost: %+v", entry)
			}
			if entry.RequestedModel != tc.requested || entry.Model != "gpt-4o" || entry.ChannelID != channels[len(channels)-1].ID || entry.Status != 2 || entry.Quota != 0 || entry.UnfundedQuota != 0 || entry.UsedChannels != strings.Join(trail, "->") {
				t.Fatalf("failed log attribution or charge is wrong: %+v", entry)
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
				t.Fatal("failed relay changed user or token quota after refund")
			}
			for _, ch := range channels {
				after, err := model.GetChannelByID(ch.ID)
				if err != nil || after.UsedQuota != ch.UsedQuota {
					t.Fatalf("failed channel was charged: %+v err=%v", after, err)
				}
			}
			var page struct {
				Items []model.Log `json:"items"`
			}
			decodeData(t, call(ListLogs, testRequest{target: "/api/log", userID: user.ID, role: common.RoleUser}), &page)
			if len(page.Items) != 1 || page.Items[0].ID != entry.ID || page.Items[0].RequestedModel != tc.requested || page.Items[0].Model != "gpt-4o" || page.Items[0].Status != 2 || page.Items[0].Quota != 0 {
				t.Fatalf("log API lost failed request metadata: %+v", page)
			}
		})
	}
}

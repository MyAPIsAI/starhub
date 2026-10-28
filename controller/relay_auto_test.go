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
}

func TestRelayAutoModelReachesUpstream(t *testing.T) {
	for _, tc := range []struct {
		name    string
		inbound dto.InboundFormat
		kind    int
	}{
		{"openai passthrough", dto.InboundOpenAI, channel.ChannelTypeOpenAI},
		{"claude passthrough", dto.InboundClaude, channel.ChannelTypeClaude},
		{"openai to claude", dto.InboundOpenAI, channel.ChannelTypeClaude},
		{"claude to openai", dto.InboundClaude, channel.ChannelTypeOpenAI},
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
			body := `{"model":"myapis-auto","messages":[{"role":"user","content":"Hello"}],"max_tokens":64,"stream":false}`
			r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Authorization", "Bearer "+token.Key)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, r)
			if rec.Code != http.StatusOK {
				t.Fatalf("relay failed: %d %s", rec.Code, rec.Body)
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

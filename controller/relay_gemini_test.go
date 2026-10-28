package controller

import (
	"encoding/json"
	"fmt"
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

func geminiTestHandler() http.Handler {
	return middleware.TokenAuth(middleware.Distribute(dto.InboundGemini)(http.HandlerFunc(RelayGemini)))
}

func geminiTestRequest(key, path, body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("x-goog-api-key", key)
	return r
}

func TestGeminiNativeRelayPreservesWireAndAttribution(t *testing.T) {
	for _, requested := range []string{"gpt-4o", "myapis-auto", "veyra-auto"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", requested, stream), func(t *testing.T) {
				newTestDB(t)
				body := `{"contents":[{"parts":[{"text":"hello"}]}],"generationConfig":{"maxOutputTokens":16},"extension":9007199254740993}`
				response := `{"candidates":[{"content":{"role":"model","parts":[{"text":"hello"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":2,"thoughtsTokenCount":3,"cachedContentTokenCount":4},"extension":9007199254740993}`
				contentType := "application/json"
				method := "generateContent"
				if stream {
					method, contentType = "streamGenerateContent?alt=sse", "text/event-stream"
					response = ": keepalive\r\nid: 9007199254740993\r\nretry: 1\r\ndata: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"Hi\"}]}}],\"usageMetadata\":{\"promptTokenCount\":5,\"candidatesTokenCount\":1}}\r\n\r\ndata: " + response + "\n\n"
				}
				var calls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					sent, _ := io.ReadAll(r.Body)
					if r.URL.RequestURI() != "/v1beta/models/gpt-4o:"+method || string(sent) != body || r.Header.Get("x-goog-api-key") != "supplier-key" || r.Header.Get("Authorization") != "" {
						t.Errorf("native request changed: %s %s %v", r.URL, sent, r.Header)
					}
					w.Header().Set("Content-Type", contentType)
					_, _ = io.WriteString(w, response)
				}))
				defer upstream.Close()
				responsesTestChannel(t, upstream.URL+"/v1beta/", channel.ChannelTypeGemini)
				if err := model.UpsertModelMeta(&model.ModelMeta{Model: "gpt-4o", Tags: "gemini"}); err != nil {
					t.Fatal(err)
				}
				user := newUser(t, "gemini-native", common.RoleUser, 1000000)
				token := mustToken(t, user.ID, 1000000)
				rec := httptest.NewRecorder()
				geminiTestHandler().ServeHTTP(rec, geminiTestRequest(token.Key, "/v1beta/models/"+requested+":"+method, body))
				logs, count, err := model.ListLogs(user.ID, 1, 10, "")
				if rec.Code != 200 || rec.Body.String() != response || calls.Load() != 1 || err != nil || count != 1 {
					t.Fatalf("native response corrupted or repeated: %d %s calls=%d logs=%+v %v", rec.Code, rec.Body, calls.Load(), logs, err)
				}
				entry := logs[0]
				after, _ := model.GetUserByID(user.ID)
				if entry.RequestedModel != requested || entry.Model != "gpt-4o" || entry.Protocol != "Gemini" || entry.PromptTokens != 10 || entry.CompletionTokens != 5 || entry.Status != 1 || after.Quota != user.Quota-entry.Quota || after.UsedQuota != entry.Quota {
					t.Fatalf("native attribution or charge lost: %+v user=%+v", entry, after)
				}
			})
		}
	}
}

func TestGeminiNativeOutcomesAndUsageFallback(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		stream         bool
		input, output  int64
		status         int
	}{
		{"zero", `{"candidates":[{"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":0,"candidatesTokenCount":0}}`, false, 0, 0, 1},
		{"text fallback", `{"candidates":[{"content":{"parts":[{"text":"12345678"}]},"finishReason":"STOP"}]}`, false, 2, 2, 1},
		{"blocked measured", `{"promptFeedback":{"blockReason":"SAFETY"},"usageMetadata":{"promptTokenCount":3}}`, false, 3, 0, 2},
		{"length", `{"candidates":[{"finishReason":"MAX_TOKENS"}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":1}}`, false, 3, 1, 1},
		{"broken SSE", "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"12345678\"}]}}]}\n\n", true, 2, 2, 2},
		{"final usage after finish", "data: {\"candidates\":[{\"finishReason\":\"STOP\"}]}\n\ndata: {\"usageMetadata\":{\"promptTokenCount\":3,\"candidatesTokenCount\":1}}\n\n", true, 3, 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			newTestDB(t)
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				ct := "application/json"
				if tc.stream {
					ct = "text/event-stream"
				}
				w.Header().Set("Content-Type", ct)
				_, _ = io.WriteString(w, tc.response)
			}))
			defer upstream.Close()
			responsesTestChannel(t, upstream.URL, channel.ChannelTypeGemini)
			responsesTestChannel(t, upstream.URL, channel.ChannelTypeGemini)
			user := newUser(t, "gemini-outcome", common.RoleUser, 1000000)
			token := mustToken(t, user.ID, 1000000)
			path := "/v1beta/models/gpt-4o:generateContent"
			if tc.stream {
				path = "/v1beta/models/gpt-4o:streamGenerateContent?alt=sse"
			}
			rec := httptest.NewRecorder()
			geminiTestHandler().ServeHTTP(rec, geminiTestRequest(token.Key, path, `{"contents":[{"parts":[{"text":"12345678"}]}],"generationConfig":{"maxOutputTokens":16}}`))
			logs, count, err := model.ListLogs(user.ID, 1, 10, "")
			if rec.Code != 200 || rec.Body.String() != tc.response || calls.Load() != 1 || err != nil || count != 1 || logs[0].PromptTokens != tc.input || logs[0].CompletionTokens != tc.output || logs[0].Status != tc.status {
				t.Fatalf("outcome lost: %d %s calls=%d logs=%+v %v", rec.Code, rec.Body, calls.Load(), logs, err)
			}
		})
	}
}

func TestGeminiNativeRejectsBeforeCharge(t *testing.T) {
	for _, tc := range []struct {
		name, path, body string
		kind, status     int
	}{
		{"unsupported channel", "gpt-4o:generateContent", `{"contents":[{"parts":[{"text":"x"}]}]}`, channel.ChannelTypeOpenAI, 503},
		{"missing contents", "gpt-4o:generateContent", `{}`, channel.ChannelTypeGemini, 400},
		{"excess output", "gpt-4o:generateContent", `{"contents":[{"parts":[{"text":"x"}]}],"generationConfig":{"maxOutputTokens":100001}}`, channel.ChannelTypeGemini, 400},
		{"stream array", "gpt-4o:streamGenerateContent", `{"contents":[{"parts":[{"text":"x"}]}]}`, channel.ChannelTypeGemini, 400},
		{"unsupported method", "gpt-4o:countTokens", `{"contents":[{"parts":[{"text":"x"}]}]}`, channel.ChannelTypeGemini, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			newTestDB(t)
			responsesTestChannel(t, "http://127.0.0.1:1", tc.kind)
			user := newUser(t, "gemini-rejected", common.RoleUser, 1000000)
			token := mustToken(t, user.ID, 1000000)
			rec := httptest.NewRecorder()
			geminiTestHandler().ServeHTTP(rec, geminiTestRequest(token.Key, "/v1beta/models/"+tc.path, tc.body))
			var body struct {
				Error struct {
					Code int `json:"code"`
				} `json:"error"`
			}
			err := json.Unmarshal(rec.Body.Bytes(), &body)
			after, _ := model.GetUserByID(user.ID)
			count, _ := model.CountLogs(user.ID)
			if rec.Code != tc.status || err != nil || body.Error.Code != tc.status || after.Quota != user.Quota || count != 0 {
				t.Fatalf("rejection failed: %d %s user=%+v logs=%d err=%v", rec.Code, rec.Body, after, count, err)
			}
		})
	}
}

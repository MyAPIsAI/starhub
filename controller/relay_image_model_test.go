package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
	"starhub/relay/channel"
	"starhub/relay/dto"
	"starhub/setting"
)

func TestImageModelAttribution(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, requested := range []string{"myapis-auto", "gpt-image-1"} {
			t.Run(fmt.Sprintf("stream=%t/%s", stream, requested), func(t *testing.T) {
				newTestDB(t)
				if err := setting.UpdatePricing(setting.OptionKeyModelPrice, `{"gpt-image-1":0.04}`); err != nil {
					t.Fatal(err)
				}
				const response = `{"data":[{"b64_json":"QUJD"},{"b64_json":"REVG"}]}`
				captured := make(chan []byte, common.RetryTimes+1)
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(r.Body)
					if err != nil || r.URL.Path != "/v1/images/generations" || r.Method != http.MethodPost {
						http.Error(w, "invalid image request", http.StatusBadRequest)
						return
					}
					captured <- body
					if stream {
						w.Header().Set("Content-Type", "text/event-stream")
						for i, data := range []string{"QUJD", "REVG"} {
							fmt.Fprintf(w, "event: image_generation.completed\ndata: {\"type\":\"image_generation.completed\",\"output_index\":%d,\"b64_json\":%q}\n\n", i, data)
						}
						_, _ = io.WriteString(w, "data: [DONE]\n\n")
						return
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, response)
				}))
				defer upstream.Close()
				ch := &model.Channel{Name: "image-review", Type: channel.ChannelTypeOpenAI, Key: "review-key", BaseURL: upstream.URL, Models: "gpt-image-1", Group: "default", Status: common.ChannelStatusEnabled, Weight: 1}
				if err := model.InsertChannel(ch); err != nil {
					t.Fatal(err)
				}
				if err := model.UpsertModelMeta(&model.ModelMeta{Model: "gpt-image-1", Tags: "image"}); err != nil {
					t.Fatal(err)
				}
				user := newUser(t, "image-review", common.RoleUser, 1000000)
				token := mustToken(t, user.ID, 1000000)
				handler := middleware.TokenAuth(middleware.Distribute(dto.InboundOpenAI)(http.HandlerFunc(RelayImageGenerations)))
				body := fmt.Sprintf(`{"model":%q,"prompt":"a cat","n":2,"stream":%t}`, requested, stream)
				r := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(body))
				r.Header.Set("Content-Type", "application/json")
				r.Header.Set("Authorization", "Bearer "+token.Key)
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, r)
				if rec.Code != http.StatusOK {
					t.Fatalf("image relay failed: %d %s", rec.Code, rec.Body)
				}
				if stream {
					if rec.Header().Get("Content-Type") != "text/event-stream" || strings.Count(rec.Body.String(), "event: image_generation.completed") != 2 {
						t.Fatalf("image events changed: %s", rec.Body)
					}
				} else if rec.Body.String() != response {
					t.Fatalf("image response changed: %s", rec.Body)
				}
				select {
				case sent := <-captured:
					var payload struct {
						Model  string `json:"model"`
						N      int    `json:"n"`
						Stream bool   `json:"stream"`
					}
					if err := json.Unmarshal(sent, &payload); err != nil || payload.Model != "gpt-image-1" || payload.N != 2 || payload.Stream != stream {
						t.Fatalf("upstream image request changed: %s, err=%v", sent, err)
					}
				default:
					t.Fatal("request did not reach upstream")
				}
				entries, total, err := model.ListLogs(user.ID, 1, 10, "gpt-image-1")
				if err != nil || total != 1 || len(entries) != 1 {
					t.Fatalf("image log missing: total=%d entries=%d err=%v", total, len(entries), err)
				}
				entry := entries[0]
				const charge = 40000 // Two delivered images at $0.04 each.
				if entry.RequestedModel != requested || entry.Model != "gpt-image-1" || entry.ChannelID != ch.ID || entry.Status != 1 || entry.Quota != charge {
					t.Fatalf("image model attribution or billing changed: %+v", entry)
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
					t.Fatal("images were not charged exactly once")
				}
				var page struct {
					Items []model.Log `json:"items"`
				}
				decodeData(t, call(ListLogs, testRequest{target: "/api/log", userID: user.ID, role: common.RoleUser}), &page)
				if len(page.Items) != 1 || page.Items[0].RequestedModel != requested || page.Items[0].Model != "gpt-image-1" {
					t.Fatalf("log API lost image models: %+v", page)
				}
			})
		}
	}
}

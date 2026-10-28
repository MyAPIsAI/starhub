package controller

import (
	"context"
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
	"starhub/service"
	"starhub/setting"
)

func TestTaskModelAttribution(t *testing.T) {
	for _, tc := range []struct {
		name       string
		requested  string
		background bool
		legacy     bool
	}{
		{"routed poll", "myapis-auto", false, false},
		{"explicit poll", "sora-2", false, false},
		{"routed background poll", "myapis-auto", true, false},
		{"legacy poll", "sora-2", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			newTestDB(t)
			if err := setting.UpdatePricing(setting.OptionKeyModelPrice, `{"sora-2":0.01}`); err != nil {
				t.Fatal(err)
			}
			captured := make(chan []byte, common.RetryTimes+1)
			var polls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.URL.Path == "/v1/videos" && r.Method == http.MethodPost:
					body, err := io.ReadAll(r.Body)
					if err != nil {
						http.Error(w, "read failed", http.StatusBadRequest)
						return
					}
					captured <- body
					_, _ = io.WriteString(w, `{"id":"video_review","status":"queued"}`)
				case r.URL.Path == "/v1/videos/video_review" && r.Method == http.MethodGet:
					polls.Add(1)
					_, _ = io.WriteString(w, `{"id":"video_review","status":"completed","progress":100,"video":{"url":"https://example.test/video.mp4"}}`)
				default:
					http.Error(w, "unexpected video request", http.StatusNotFound)
				}
			}))
			defer upstream.Close()
			ch := &model.Channel{Name: "task-review", Type: channel.ChannelTypeOpenAI, Key: "review-key", BaseURL: upstream.URL, Models: "sora-2", Group: "default", Status: common.ChannelStatusEnabled, Weight: 1}
			if err := model.InsertChannel(ch); err != nil {
				t.Fatal(err)
			}
			if err := model.UpsertModelMeta(&model.ModelMeta{Model: "sora-2", Tags: "video"}); err != nil {
				t.Fatal(err)
			}
			user := newUser(t, "task-review", common.RoleUser, 1000000)
			token := mustToken(t, user.ID, 1000000)
			handler := middleware.TokenAuth(middleware.Distribute(dto.InboundOpenAI)(http.HandlerFunc(RelayTaskSubmitVideo)))
			body := fmt.Sprintf(`{"model":%q,"prompt":"a cat","seconds":8,"n":2}`, tc.requested)
			r := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Authorization", "Bearer "+token.Key)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, r)
			var submitted dto.TaskSubmitResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &submitted); err != nil || rec.Code != http.StatusOK || submitted.ID == "" {
				t.Fatalf("task submit failed: %d %s, err=%v", rec.Code, rec.Body, err)
			}
			id, err := strconv.ParseInt(submitted.ID, 10, 64)
			if err != nil {
				t.Fatal(err)
			}
			task, err := model.GetTaskByID(id)
			if err != nil {
				t.Fatal(err)
			}
			if task.RequestedModel != tc.requested || task.Model != "sora-2" || task.Quota != 80000 {
				t.Fatalf("persisted task lost model attribution or pre-charge: %+v", task)
			}
			select {
			case sent := <-captured:
				var payload dto.TaskRequest
				if err := json.Unmarshal(sent, &payload); err != nil || payload.Model != "sora-2" || payload.Seconds != 8 || payload.N != 2 {
					t.Fatalf("upstream task request changed: %s, err=%v", sent, err)
				}
			default:
				t.Fatal("request did not reach upstream")
			}
			if tc.legacy {
				if err := model.DB.Model(&model.Task{}).Where("id = ?", id).Update("requested_model", "").Error; err != nil {
					t.Fatal(err)
				}
			}
			if tc.background {
				service.PollPendingTasksOnce(context.Background())
				service.PollPendingTasksOnce(context.Background())
			} else {
				for range 2 {
					r := httptest.NewRequest(http.MethodGet, "/v1/video/generations/"+submitted.ID, nil)
					r.SetPathValue("id", submitted.ID)
					r.Header.Set("Authorization", "Bearer "+token.Key)
					rec := httptest.NewRecorder()
					middleware.TokenAuth(http.HandlerFunc(RelayTaskGetVideo)).ServeHTTP(rec, r)
					if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"succeeded"`) {
						t.Fatalf("task poll failed: %d %s", rec.Code, rec.Body)
					}
				}
			}
			entries, total, err := model.ListLogs(user.ID, 1, 10, "sora-2")
			if err != nil || total != 1 || len(entries) != 1 || polls.Load() != 1 {
				t.Fatalf("expected one settled log and upstream poll: total=%d entries=%d polls=%d err=%v", total, len(entries), polls.Load(), err)
			}
			entry := entries[0]
			if entry.RequestedModel != tc.requested || entry.Model != "sora-2" || entry.Quota != 80000 || entry.ChannelID != ch.ID {
				t.Fatalf("terminal log lost model attribution or changed billing: %+v", entry)
			}
			afterUser, err := model.GetUserByID(user.ID)
			if err != nil {
				t.Fatal(err)
			}
			afterToken, err := model.GetTokenByID(token.ID)
			if err != nil {
				t.Fatal(err)
			}
			if afterUser.Quota != user.Quota-80000 || afterUser.UsedQuota != user.UsedQuota+80000 || afterToken.RemainQuota != token.RemainQuota-80000 || afterToken.UsedQuota != token.UsedQuota+80000 {
				t.Fatal("task was not settled once at the actual model price")
			}
			var page struct {
				Items []model.Log `json:"items"`
			}
			decodeData(t, call(ListLogs, testRequest{target: "/api/log", userID: user.ID, role: common.RoleUser}), &page)
			if len(page.Items) != 1 || page.Items[0].RequestedModel != tc.requested || page.Items[0].Model != "sora-2" {
				t.Fatalf("log API lost task models: %+v", page)
			}
		})
	}
}

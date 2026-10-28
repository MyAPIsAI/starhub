package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"starhub/common"
	"starhub/model"
	"starhub/relay"
	"starhub/relay/channel"
	"starhub/relay/dto"
	"starhub/service"
	"starhub/setting"
)

func TestConcurrentTaskSettlement(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     string
		actual     int64
		logStatus  int
		expression string
	}{
		{"completed", "completed", 80000, 1, ""},
		{"failed", "failed", 0, 2, ""},
		{"cancelled", "cancelled", 40000, 1, ""},
		{"free success", "completed", 0, 1, `{"sora-2":"0"}`},
		{"charged failure", "failed", 20000, 1, `{"sora-2":"task_status == 1 ? seconds * 0.01 * n : 0.04"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			newTestDB(t)
			if err := setting.UpdatePricing(setting.OptionKeyModelPrice, `{"sora-2":0.01}`); err != nil {
				t.Fatal(err)
			}
			if tc.expression != "" {
				if err := setting.UpdatePricing(setting.OptionKeyModelPriceExpr, tc.expression); err != nil {
					t.Fatal(err)
				}
			}
			arrived := make(chan struct{}, 2)
			release := make(chan struct{})
			var once sync.Once
			releaseUpstream := func() { once.Do(func() { close(release) }) }
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/videos/video_concurrent" {
					http.Error(w, "unexpected poll", http.StatusNotFound)
					return
				}
				arrived <- struct{}{}
				<-release
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"id":"video_concurrent","status":%q,"progress":100}`, tc.status)
			}))
			defer upstream.Close()
			defer releaseUpstream()
			ch := &model.Channel{Name: "concurrent-task", Type: channel.ChannelTypeOpenAI, Key: "review-key", BaseURL: upstream.URL, Models: "sora-2", Group: "default", Status: common.ChannelStatusEnabled, Weight: 1}
			if err := model.InsertChannel(ch); err != nil {
				t.Fatal(err)
			}
			user := newUser(t, "concurrent-task", common.RoleUser, 1000000)
			token := mustToken(t, user.ID, 1000000)
			pre, err := relay.PreConsumeQuotaAmount(user.ID, token.ID, false, 80000)
			if err != nil {
				t.Fatal(err)
			}
			task := &model.Task{
				TaskID: "video_concurrent", Platform: "sora", Action: dto.TaskActionVideo,
				Model: "sora-2", RequestedModel: "myapis-auto", UserID: user.ID,
				TokenID: token.ID, ChannelID: ch.ID, Group: "default", UserGroup: "default",
				Quota: pre, RequestMeta: `{"model":"sora-2","prompt":"a cat","seconds":8,"n":2}`,
			}
			if err := model.InsertTask(task); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			done := make(chan error, 2)
			go func() {
				_, err := service.PollTask(ctx, task)
				done <- err
			}()
			go func() {
				service.PollPendingTasksOnce(ctx)
				done <- nil
			}()
			// Both callers must load the active task before either can finalize it.
			for range 2 {
				select {
				case <-arrived:
				case <-ctx.Done():
					t.Fatal("concurrent callers did not both reach upstream")
				}
			}
			releaseUpstream()
			for range 2 {
				select {
				case err := <-done:
					if err != nil {
						t.Fatal(err)
					}
				case <-ctx.Done():
					t.Fatal("concurrent pollers did not finish")
				}
			}
			logs, total, err := model.ListLogs(user.ID, 1, 10, "sora-2")
			if err != nil || total != 1 || len(logs) != 1 || logs[0].Quota != tc.actual || logs[0].RequestedModel != "myapis-auto" || logs[0].Status != tc.logStatus {
				t.Fatalf("expected one terminal settlement: total=%d logs=%+v err=%v", total, logs, err)
			}
			var page struct {
				Items []model.Log `json:"items"`
			}
			decodeData(t, call(ListLogs, testRequest{target: "/api/log", userID: user.ID, role: common.RoleUser}), &page)
			if len(page.Items) != 1 || page.Items[0].ID != logs[0].ID || page.Items[0].Status != tc.logStatus || page.Items[0].Quota != tc.actual {
				t.Fatalf("log API changed terminal status or charge: %+v", page)
			}
			days, err := model.SummarizeLogsByDay(user.ID, 0)
			var successes, failures int64
			if tc.logStatus == 2 {
				failures = 1
			} else {
				successes = 1
			}
			if err != nil || len(days) != 1 || days[0].Requests != 1 || days[0].SuccessCount != successes || days[0].FailedCount != failures || days[0].Quota != tc.actual {
				t.Fatalf("terminal request classified incorrectly: %+v err=%v", days, err)
			}
			afterUser, err := model.GetUserByID(user.ID)
			if err != nil {
				t.Fatal(err)
			}
			afterToken, err := model.GetTokenByID(token.ID)
			if err != nil {
				t.Fatal(err)
			}
			afterChannel, err := model.GetChannelByID(ch.ID)
			if err != nil {
				t.Fatal(err)
			}
			if afterUser.Quota != user.Quota-tc.actual || afterUser.UsedQuota != user.UsedQuota+tc.actual || afterToken.RemainQuota != token.RemainQuota-tc.actual || afterToken.UsedQuota != token.UsedQuota+tc.actual || afterChannel.UsedQuota != ch.UsedQuota+tc.actual {
				t.Fatalf("duplicate charge or refund: user=%+v token=%+v channel=%+v", afterUser, afterToken, afterChannel)
			}
			stored, err := model.GetTaskByID(task.ID)
			if err != nil || !dto.IsTerminalTaskStatus(stored.Status) {
				t.Fatalf("task did not finalize: %+v err=%v", stored, err)
			}
			// A later background sweep must leave the settled record untouched.
			service.PollPendingTasksOnce(ctx)
			if count, err := model.CountLogs(user.ID); err != nil || count != 1 {
				t.Fatalf("terminal task settled again: count=%d err=%v", count, err)
			}
		})
	}
}

func TestInvalidSettlementLogStatusPreservesQuota(t *testing.T) {
	newTestDB(t)
	user := newUser(t, "invalid-settlement-status", common.RoleUser, 1000000)
	token := mustToken(t, user.ID, 1000000)
	for _, tc := range []struct {
		status int
		actual int64
	}{
		{0, 0},
		{3, 0},
		{2, 1000},
	} {
		if err := relay.PostConsumeQuotaAmountWithStatus(user.ID, token.ID, "review", false,
			0, "sora-2", tc.actual, 80000, "", 0, tc.status, "myapis-auto"); err == nil {
			t.Fatalf("invalid log status accepted: status=%d actual=%d", tc.status, tc.actual)
		}
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
		t.Fatal("invalid log status changed balances or refunded an unreserved amount")
	}
	if count, err := model.CountLogs(user.ID); err != nil || count != 0 {
		t.Fatalf("invalid log status created records: count=%d err=%v", count, err)
	}
}

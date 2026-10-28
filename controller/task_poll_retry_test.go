package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"gorm.io/gorm"
	"starhub/common"
	"starhub/model"
	"starhub/relay"
	"starhub/relay/channel"
	"starhub/service"
	"starhub/setting"
)

func retryTaskFixture(t *testing.T, upstream string) (*model.User, *model.Token, *model.Task) {
	t.Helper()
	newTestDB(t)
	if err := setting.UpdatePricing(setting.OptionKeyModelPrice, `{"sora-2":0.01}`); err != nil {
		t.Fatal(err)
	}
	ch := &model.Channel{Name: "retry-poll", Type: channel.ChannelTypeOpenAI, Models: "sora-2", Group: "default", Status: common.ChannelStatusEnabled, Key: "local-review", BaseURL: upstream}
	if err := model.InsertChannel(ch); err != nil {
		t.Fatal(err)
	}
	u := newUser(t, "retry-poll", common.RoleUser, 1000000)
	tk := mustToken(t, u.ID, 1000000)
	pre, err := relay.PreConsumeQuotaAmount(u.ID, tk.ID, false, 80000)
	if err != nil {
		t.Fatal(err)
	}
	task := &model.Task{TaskID: "retry-poll", Model: "sora-2", Platform: "sora", Action: "video", RequestedModel: "myapis-auto", Protocol: "OpenAI", UserID: u.ID, TokenID: tk.ID, ChannelID: ch.ID, Group: "default", UserGroup: "default", Quota: pre, RequestMeta: `{"model":"sora-2","seconds":8,"n":2}`}
	if err := model.InsertTask(task); err != nil {
		t.Fatal(err)
	}
	return u, tk, task
}

func assertTaskReserved(t *testing.T, userID, tokenID, taskID int64) {
	t.Helper()
	u, err := model.GetUserByID(userID)
	if err != nil {
		t.Fatal(err)
	}
	tk, err := model.GetTokenByID(tokenID)
	if err != nil {
		t.Fatal(err)
	}
	task, err := model.GetTaskByID(taskID)
	if err != nil {
		t.Fatal(err)
	}
	count, err := model.CountLogs(userID)
	if err != nil {
		t.Fatal(err)
	}
	if u.Quota != 920000 || tk.RemainQuota != 920000 || u.UsedQuota != 0 || tk.UsedQuota != 0 || count != 0 || task.Status != "submitted" || task.PendingStatus != "" || task.CompleteTime != 0 {
		t.Fatalf("temporary error finalized or refunded the task: user=%+v token=%+v task=%+v logs=%d", u, tk, task, count)
	}
}

func assertTaskChargedOnce(t *testing.T, task *model.Task) {
	t.Helper()
	for range 2 {
		if _, err := service.PollTask(context.Background(), task); err != nil {
			t.Fatal(err)
		}
	}
	u, err := model.GetUserByID(task.UserID)
	if err != nil {
		t.Fatal(err)
	}
	tk, err := model.GetTokenByID(task.TokenID)
	if err != nil {
		t.Fatal(err)
	}
	logs, total, err := model.ListLogs(task.UserID, 1, 10, "")
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != "succeeded" || task.PendingStatus != "" || u.Quota != 920000 || tk.RemainQuota != 920000 || u.UsedQuota != 80000 || tk.UsedQuota != 80000 || total != 1 || len(logs) != 1 || logs[0].Quota != 80000 || logs[0].Status != 1 {
		t.Fatalf("task did not recover exactly once: task=%+v user=%+v token=%+v logs=%+v", task, u, tk, logs)
	}
}

func TestTaskPollKeepsUncertainHTTPResultsPending(t *testing.T) {
	for _, status := range []int{400, 401, 403, 408, 422, 429, 500, 503} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			var response atomic.Int32
			response.Store(int32(status))
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(int(response.Load()))
				_, _ = w.Write([]byte(`{"id":"retry-poll","status":"completed","progress":100}`))
			}))
			defer upstream.Close()
			u, tk, task := retryTaskFixture(t, upstream.URL)
			if _, err := service.PollTask(context.Background(), task); err == nil {
				t.Fatal("uncertain upstream result was not reported")
			}
			assertTaskReserved(t, u.ID, tk.ID, task.ID)
			response.Store(200)
			service.PollPendingTasksOnce(context.Background())
			reloaded, err := model.GetTaskByID(task.ID)
			if err != nil {
				t.Fatal(err)
			}
			assertTaskChargedOnce(t, reloaded)
			if calls.Load() != 2 {
				t.Fatalf("unexpected upstream polls: %d", calls.Load())
			}
		})
	}
}

func TestTaskPollRecoversFromChannelQueryFailure(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"id":"retry-poll","status":"completed","progress":100}`))
	}))
	defer upstream.Close()
	u, tk, task := retryTaskFixture(t, upstream.URL)
	want := errors.New("temporary channel query failure")
	callback := "review:temporary_channel_query"
	if err := model.DB.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "channels" {
			tx.AddError(want)
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { model.DB.Callback().Query().Remove(callback) })
	if _, err := service.PollTask(context.Background(), task); !errors.Is(err, want) {
		t.Fatalf("channel query error lost: %v", err)
	}
	assertTaskReserved(t, u.ID, tk.ID, task.ID)
	if calls.Load() != 0 {
		t.Fatal("channel query failure contacted upstream")
	}
	model.DB.Callback().Query().Remove(callback)
	assertTaskChargedOnce(t, task)
	if calls.Load() != 1 {
		t.Fatal("recovery polled more than once")
	}
}

func TestTaskPollRefundsDefinitiveMissingResults(t *testing.T) {
	for _, status := range []int{404, 410} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
			defer upstream.Close()
			u, _, task := retryTaskFixture(t, upstream.URL)
			for range 2 {
				if _, err := service.PollTask(context.Background(), task); err != nil {
					t.Fatal(err)
				}
			}
			after, err := model.GetUserByID(u.ID)
			if err != nil {
				t.Fatal(err)
			}
			logs, total, err := model.ListLogs(u.ID, 1, 10, "")
			if err != nil {
				t.Fatal(err)
			}
			if after.Quota != 1000000 || task.Status != "failed" || total != 1 || logs[0].Status != 2 || logs[0].Quota != 0 {
				t.Fatalf("definitive failure settlement: task=%+v user=%+v logs=%+v", task, after, logs)
			}
		})
	}
}

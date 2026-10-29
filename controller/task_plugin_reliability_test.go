package controller

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
	"starhub/model"
	"starhub/pkg/taskplugin"
	"starhub/relay"
	"starhub/relay/channel"
	"starhub/service"
	"starhub/setting"
)

type pluginPollingFixture struct {
	task    *model.Task
	user    *model.User
	token   *model.Token
	channel *model.Channel
}

func newPluginPollingFixture(t *testing.T, handler http.HandlerFunc, submitting bool) pluginPollingFixture {
	t.Helper()
	newTestDB(t)
	if err := setting.UpdatePricing(setting.OptionKeyModelPrice, `{"sora-2":0.01}`); err != nil {
		t.Fatal(err)
	}
	plugin := saveExamplePlugin(t, "")
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	ch := &model.Channel{Name: "reliability", Type: channel.ChannelTypeTaskPlugin, TaskPluginID: plugin.ID, Key: "test-key", BaseURL: server.URL, Models: "sora-2", Group: "default", Status: 1, Weight: 1}
	if err := model.InsertChannel(ch); err != nil {
		t.Fatal(err)
	}
	user := newUser(t, "plugin-reliability", 1, 1000000)
	token := mustToken(t, user.ID, 1000000)
	snapshot, err := json.Marshal(relay.PluginTaskPricingSnapshot{PriceData: *relay.NewPriceData("sora-2", "default"), Usage: taskplugin.Usage{Seconds: 8, N: 2}})
	if err != nil {
		t.Fatal(err)
	}
	task := &model.Task{Status: model.TaskStatusSubmitting, TaskPluginID: plugin.ID, Platform: "openai-video", ChannelID: ch.ID, UserID: user.ID, TokenID: token.ID, Model: "sora-2", Group: "default", Quota: 80000,
		RequestMeta: `{"model":"sora-2","prompt":"review","seconds":8,"n":2}`, PricingMeta: string(snapshot)}
	if err := model.ReservePluginTask(task, ch); err != nil {
		t.Fatal(err)
	}
	if !submitting {
		if err := model.CompletePluginSubmission(task.ID, "review-job"); err != nil {
			t.Fatal(err)
		}
		task.TaskID, task.Status = "review-job", "submitted"
	}
	return pluginPollingFixture{task: task, user: user, token: token, channel: ch}
}

func TestPluginSuccessWaitsForDeliverable(t *testing.T) {
	var deliver atomic.Bool
	f := newPluginPollingFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if deliver.Load() {
			_, _ = io.WriteString(w, `{"status":"completed","seconds":6,"video":{"url":"https://example.test/delivered.mp4"}}`)
		} else {
			_, _ = io.WriteString(w, `{"status":"completed","progress":100}`)
		}
	}, false)
	if _, err := service.PollTask(context.Background(), f.task); err == nil {
		t.Fatal("empty success accepted")
	}
	stored, _ := model.GetTaskByID(f.task.ID)
	count, _ := model.CountLogs(f.user.ID)
	user, _ := model.GetUserByID(f.user.ID)
	if stored.Status != "submitted" || stored.PendingStatus != "" || count != 0 || user.UsedQuota != 0 || user.Quota != f.user.Quota-80000 {
		t.Fatalf("undelivered result was settled: %+v %+v logs=%d", stored, user, count)
	}
	deliver.Store(true)
	if _, err := service.PollTask(context.Background(), f.task); err != nil {
		t.Fatal(err)
	}
	if f.task.Status != "succeeded" || len(model.TaskOutputURLs(f.task.ResultMeta)) != 1 {
		t.Fatalf("delivery lost: %+v", f.task)
	}
	user, _ = model.GetUserByID(f.user.ID)
	if user.Quota != f.user.Quota-60000 {
		t.Fatal("delivered usage was not settled")
	}
}

func TestPluginMissingTaskConfirmationAndRefund(t *testing.T) {
	for _, status := range []int{404, 410} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			var interrupted atomic.Bool
			f := newPluginPollingFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if interrupted.Load() {
					w.WriteHeader(429)
				} else {
					w.WriteHeader(status)
				}
				_, _ = io.WriteString(w, `{}`)
			}, false)
			f.task.SubmitTime = time.Now().Add(-service.PluginMissingGrace - time.Minute).Unix()
			if err := model.DB.Model(f.task).Update("submit_time", f.task.SubmitTime).Error; err != nil {
				t.Fatal(err)
			}
			for range 3 {
				if _, err := service.PollTask(context.Background(), f.task); err == nil {
					t.Fatal("first rapid misses must remain pending")
				}
			}
			stored, _ := model.GetTaskByID(f.task.ID)
			if stored.PluginMissingCount != 1 {
				t.Fatalf("rapid polls accelerated confirmation: %d", stored.PluginMissingCount)
			}
			interrupted.Store(true)
			_, _ = service.PollTask(context.Background(), f.task)
			stored, _ = model.GetTaskByID(f.task.ID)
			if stored.PluginMissingCount != 0 {
				t.Fatal("transient response did not reset consecutive misses")
			}
			interrupted.Store(false)
			for i := range 3 {
				if err := model.DB.Model(f.task).UpdateColumn("plugin_missing_time", time.Now().Add(-2*time.Minute).Unix()).Error; err != nil {
					t.Fatal(err)
				}
				_, err := service.PollTask(context.Background(), f.task)
				if i < 2 && err == nil {
					t.Fatal("missing response ended task before confirmation")
				}
				if i == 2 && err != nil {
					t.Fatal(err)
				}
			}
			user, _ := model.GetUserByID(f.user.ID)
			if f.task.Status != "failed" || user.Quota != f.user.Quota {
				t.Fatal("missing task did not terminate and refund")
			}
			if _, err := service.PollTask(context.Background(), f.task); err != nil {
				t.Fatal(err)
			}
			count, _ := model.CountLogs(f.user.ID)
			if count != 1 {
				t.Fatal("missing task settled more than once")
			}
			if err := model.DeleteChannel(f.channel.ID); err != nil {
				t.Fatalf("expired task still locks channel: %v", err)
			}
		})
	}
}

func TestPluginMissingGraceAndCompletionDeadline(t *testing.T) {
	f := newPluginPollingFixture(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404); _, _ = io.WriteString(w, `{}`) }, false)
	for range 3 {
		if err := model.DB.Model(f.task).UpdateColumn("plugin_missing_time", 0).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := service.PollTask(context.Background(), f.task); err == nil {
			t.Fatal("young task was not allowed its grace period")
		}
	}
	if f.task.Status != "submitted" {
		t.Fatal("young task terminated")
	}
	f.task.SubmitTime = time.Now().Add(-service.PluginTaskTimeout - time.Minute).Unix()
	if err := model.DB.Model(f.task).UpdateColumn("submit_time", f.task.SubmitTime).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.PollTask(context.Background(), f.task); err != nil {
		t.Fatal(err)
	}
	user, _ := model.GetUserByID(f.user.ID)
	if f.task.Status != "failed" || user.Quota != f.user.Quota {
		t.Fatal("task deadline did not release reservation")
	}
}

func TestPluginAdmissionSurvivesRestartBeforeSending(t *testing.T) {
	var calls atomic.Int32
	f := newPluginPollingFixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		t.Error("abandoned submission must not be sent again")
	}, true)
	if _, err := service.PollTask(context.Background(), f.task); err != nil {
		t.Fatal(err)
	}
	if f.task.Status != model.TaskStatusSubmitting {
		t.Fatal("active submission was prematurely terminated")
	}
	f.task.SubmitTime = time.Now().Add(-service.PluginSubmissionTimeout - time.Minute).Unix()
	if err := model.DB.Model(f.task).UpdateColumn("submit_time", f.task.SubmitTime).Error; err != nil {
		t.Fatal(err)
	}
	var dbPath string
	if err := model.DB.Raw("PRAGMA database_list").Row().Scan(new(int), new(string), &dbPath); err != nil {
		t.Fatal(err)
	}
	if err := model.CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := model.InitDB(dbPath); err != nil {
		t.Fatal(err)
	}
	stored, err := model.GetTaskByID(f.task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != model.TaskStatusSubmitting || stored.Quota != 80000 {
		t.Fatal("reservation lost its task on restart")
	}
	for range 2 {
		if _, err := service.PollTask(context.Background(), stored); err != nil {
			t.Fatal(err)
		}
	}
	user, _ := model.GetUserByID(f.user.ID)
	token, _ := model.GetTokenByID(f.token.ID)
	count, _ := model.CountLogs(f.user.ID)
	if stored.Status != "failed" || user.Quota != f.user.Quota || token.RemainQuota != f.token.RemainQuota || count != 1 || calls.Load() != 0 {
		t.Fatal("abandoned submission refund was not recovered exactly once")
	}
}

func TestPluginAdmissionRollbackOnRecordFailure(t *testing.T) {
	f := newPluginPollingFixture(t, func(http.ResponseWriter, *http.Request) {}, true)
	before, _ := model.GetUserByID(f.user.ID)
	const callback = "plugin:admission-failure"
	if err := model.DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "Task" {
			tx.AddError(errors.New("injected task insert failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer model.DB.Callback().Create().Remove(callback)
	another := *f.task
	another.ID = 0
	if err := model.ReservePluginTask(&another, f.channel); err == nil {
		t.Fatal("failed task insert succeeded")
	}
	after, _ := model.GetUserByID(f.user.ID)
	token, _ := model.GetTokenByID(f.token.ID)
	var count int64
	model.DB.Model(&model.Task{}).Count(&count)
	if after.Quota != before.Quota || token.RemainQuota != f.token.RemainQuota-80000 || count != 1 {
		t.Fatal("failed admission leaked a reservation")
	}
}

func TestPluginSubmitIDPersistenceFailureRefundsDurably(t *testing.T) {
	f := newPluginPollingFixture(t, func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, `{"id":"accepted"}`) }, true)
	before, _ := model.GetUserByID(f.user.ID)
	const callback = "plugin:id-write-failure"
	if err := model.DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "Task" {
			if fields, ok := tx.Statement.Dest.(map[string]any); ok {
				if _, attaching := fields["task_id"]; attaching {
					tx.AddError(errors.New("injected ID write failure"))
				}
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer model.DB.Callback().Update().Remove(callback)
	response := submitExamplePlugin(t, f.token)
	id, err := strconv.ParseInt(response.Header().Get("X-StarHub-Task-ID"), 10, 64)
	if err != nil {
		t.Fatal("failed submit did not expose its recovery ID")
	}
	stored, err := model.GetTaskByID(id)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := model.GetUserByID(f.user.ID)
	if response.Code != 500 || stored.Status != "failed" || stored.PendingStatus != "" || after.Quota != before.Quota {
		t.Fatalf("ID write failure leaked funds: status=%d task=%+v", response.Code, stored)
	}
}

func TestPluginSubmissionRefundFailureRecoversWithoutResending(t *testing.T) {
	var calls atomic.Int32
	f := newPluginPollingFixture(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); _, _ = io.WriteString(w, `invalid-json`) }, true)
	before, _ := model.GetUserByID(f.user.ID)
	const callback = "plugin:submission-refund-failure"
	if err := model.DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "Log" {
			tx.AddError(errors.New("injected refund log failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer model.DB.Callback().Create().Remove(callback)
	response := submitExamplePlugin(t, f.token)
	id, err := strconv.ParseInt(response.Header().Get("X-StarHub-Task-ID"), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := model.GetTaskByID(id)
	if err != nil {
		t.Fatal(err)
	}
	if response.Code != 502 || stored.Status != model.TaskStatusSubmitting || stored.PendingStatus != "failed" {
		t.Fatalf("lost refund intent: %+v", stored)
	}
	if err := model.DB.Callback().Create().Remove(callback); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := service.PollTask(context.Background(), stored); err != nil {
			t.Fatal(err)
		}
	}
	after, _ := model.GetUserByID(f.user.ID)
	count, _ := model.CountLogs(f.user.ID)
	if after.Quota != before.Quota || stored.Status != "failed" || calls.Load() != 1 || count != 1 {
		t.Fatal("submission refund did not recover exactly once")
	}
}

func TestPluginAdmissionRejectsChangedChannelWithoutReserving(t *testing.T) {
	f := newPluginPollingFixture(t, func(http.ResponseWriter, *http.Request) {}, false)
	before, _ := model.GetUserByID(f.user.ID)
	another := *f.task
	another.ID = 0
	another.Status = model.TaskStatusSubmitting
	another.TaskID = ""
	stale := *f.channel
	stale.Key = "stale-key"
	if err := model.ReservePluginTask(&another, &stale); !errors.Is(err, model.ErrTaskPluginChannelChanged) {
		t.Fatalf("changed channel accepted: %v", err)
	}
	after, _ := model.GetUserByID(f.user.ID)
	if after.Quota != before.Quota {
		t.Fatal("channel revalidation leaked reservation")
	}
}

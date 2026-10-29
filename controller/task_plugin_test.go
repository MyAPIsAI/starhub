package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"starhub/common"
	"starhub/middleware"
	"starhub/model"
	"starhub/pkg/taskplugin"
	"starhub/relay"
	"starhub/relay/channel"
	"starhub/relay/dto"
	"starhub/service"
	"starhub/setting"
)

func saveExamplePlugin(t *testing.T, suffix string) *model.TaskPlugin {
	t.Helper()
	source, err := os.ReadFile("../plugins/tasks/openai-video.js")
	if err != nil {
		t.Fatal(err)
	}
	p, err := taskplugin.Compile(context.Background(), string(source)+suffix)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := model.SaveTaskPlugin(string(source)+suffix, p.Meta)
	if err != nil {
		t.Fatal(err)
	}
	return stored
}

func submitExamplePlugin(t *testing.T, token *model.Token) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest("POST", "/v1/video/generations", strings.NewReader(`{"model":"sora-2","prompt":"A cat","seconds":8,"n":2,"extra":{"seed":123}}`))
	r.Header.Set("Authorization", "Bearer "+token.Key)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	middleware.TokenAuth(middleware.Distribute(dto.InboundOpenAI)(http.HandlerFunc(RelayTaskSubmitVideo))).ServeHTTP(w, r)
	return w
}

func TestPluginTaskLifecyclePinnedVersionAndRecovery(t *testing.T) {
	for _, terminal := range []string{"completed", "failed", "cancelled"} {
		t.Run(terminal, func(t *testing.T) {
			newTestDB(t)
			if err := setting.UpdatePricing(setting.OptionKeyModelPrice, `{"sora-2":0.01}`); err != nil {
				t.Fatal(err)
			}
			stored := saveExamplePlugin(t, "")
			var submits, polls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer upstream-secret" {
					t.Error("wrong upstream key")
				}
				w.Header().Set("Content-Type", "application/json")
				if r.Method == "POST" && r.URL.Path == "/v1/videos" {
					submits.Add(1)
					body, _ := io.ReadAll(r.Body)
					if strings.Contains(string(body), "upstream-secret") {
						t.Error("credential in body")
					}
					_, _ = io.WriteString(w, `{"id":"job-v1"}`)
					return
				}
				if r.Method != "GET" || r.URL.Path != "/v1/videos/job-v1" {
					t.Errorf("unexpected poll: %s %s", r.Method, r.URL.Path)
				}
				polls.Add(1)
				_, _ = fmt.Fprintf(w, `{"status":%q,"seconds":6,"video":{"url":"https://example.test/video.mp4"}}`, terminal)
			}))
			defer upstream.Close()
			ch := &model.Channel{Name: "plugin", Type: channel.ChannelTypeTaskPlugin, TaskPluginID: stored.ID, Key: "upstream-secret", BaseURL: upstream.URL, Models: "sora-2", Group: "default", Status: common.ChannelStatusEnabled, Weight: 1}
			if err := model.InsertChannel(ch); err != nil {
				t.Fatal(err)
			}
			user := newUser(t, "plugin-user", common.RoleUser, 1000000)
			token := mustToken(t, user.ID, 1000000)
			w := submitExamplePlugin(t, token)
			var response dto.TaskSubmitResponse
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || w.Code != 200 || response.ID == "" {
				t.Fatalf("submit: %d %s %v", w.Code, w.Body, err)
			}
			var task model.Task
			if err := model.DB.First(&task, "id = ?", response.ID).Error; err != nil {
				t.Fatal(err)
			}
			if task.TaskPluginID != stored.ID || task.Quota != 80000 || !strings.Contains(task.RequestMeta, `"seed":123`) {
				t.Fatalf("snapshot incorrect: %+v", task)
			}
			if err := model.DeleteChannel(ch.ID); err == nil {
				t.Fatal("pending channel deleted")
			}
			originalKey := ch.Key
			ch.Key = "other-account"
			if err := model.UpdateChannel(ch); err == nil {
				t.Fatal("pending credentials changed")
			}
			ch.Key = originalKey
			v2 := saveExamplePlugin(t, `meta.version="2.0.0"; buildQuery=function(){throw new Error("V2 must not query V1");};`)
			ch.TaskPluginID = v2.ID
			if err := model.UpdateChannel(ch); err != nil {
				t.Fatal(err)
			}
			if err := model.SetTaskPluginEnabled(stored.ID, false); err != nil {
				t.Fatal(err)
			}
			if err := setting.UpdatePricing(setting.OptionKeyModelPrice, `{"sora-2":100}`); err != nil {
				t.Fatal(err)
			}
			// Reopen storage to verify pinned identities survive a database restart.
			sqlDB, _ := model.DB.DB()
			var dbPath string
			if err := model.DB.Raw("PRAGMA database_list").Row().Scan(new(int), new(string), &dbPath); err != nil {
				t.Fatal(err)
			}
			if err := sqlDB.Close(); err != nil {
				t.Fatal(err)
			}
			if err := model.InitDB(dbPath); err != nil {
				t.Fatal(err)
			}
			if _, err := service.PollTask(context.Background(), &task); err != nil {
				t.Fatal(err)
			}
			if _, err := service.PollTask(context.Background(), &task); err != nil {
				t.Fatal(err)
			}
			actual := int64(60000)
			if terminal != "completed" {
				actual = 0
			}
			after, _ := model.GetUserByID(user.ID)
			if after.Quota != user.Quota-actual || submits.Load() != 1 || polls.Load() != 1 {
				t.Fatalf("wrong settlement: quota=%d submits=%d polls=%d", after.Quota, submits.Load(), polls.Load())
			}
			logs, total, err := model.ListLogs(user.ID, 1, 10, "")
			if err != nil || total != 1 || logs[0].Quota != actual {
				t.Fatalf("settlement logs: %+v %d %v", logs, total, err)
			}
		})
	}
}

func TestPluginTaskDoesNotRetryOrChargeInvalidSubmission(t *testing.T) {
	for _, mode := range []string{"500", "bad-json", "missing-id", "bad-usage", "no-price", "redirect"} {
		t.Run(mode, func(t *testing.T) {
			newTestDB(t)
			if mode != "no-price" {
				if err := setting.UpdatePricing(setting.OptionKeyModelPrice, `{"sora-2":0.01}`); err != nil {
					t.Fatal(err)
				}
			}
			suffix := ""
			if mode == "bad-usage" {
				suffix = `prepareSubmit=function(){return {request:{method:"POST",path:"/v1/videos",body:{}},usage:{seconds:-1,n:1}};};`
			}
			plugin := saveExamplePlugin(t, suffix)
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if mode == "bad-json" {
					_, _ = io.WriteString(w, "not-json")
					return
				}
				if mode == "500" {
					w.WriteHeader(500)
				}
				if mode == "redirect" {
					w.Header().Set("Location", "https://example.test")
					w.WriteHeader(307)
				}
				_, _ = io.WriteString(w, `{}`)
			}))
			defer server.Close()
			for i := range 2 {
				if err := model.InsertChannel(&model.Channel{Name: fmt.Sprint(i), Type: channel.ChannelTypeTaskPlugin, TaskPluginID: plugin.ID, Key: "key", BaseURL: server.URL, Models: "sora-2", Group: "default", Status: 1, Weight: 1}); err != nil {
					t.Fatal(err)
				}
			}
			user := newUser(t, "no-retry", common.RoleUser, 1000000)
			token := mustToken(t, user.ID, 1000000)
			w := submitExamplePlugin(t, token)
			if w.Code == 200 {
				t.Fatal("invalid submission accepted")
			}
			want := int32(1)
			if mode == "bad-usage" || mode == "no-price" {
				want = 0
			}
			if calls.Load() != want {
				t.Fatalf("upstream called %d times, want %d", calls.Load(), want)
			}
			after, _ := model.GetUserByID(user.ID)
			if after.Quota != user.Quota {
				t.Fatal("failed submit leaked reservation")
			}
		})
	}
}

func TestPluginPollingTransientErrorsAndSettlementRetry(t *testing.T) {
	newTestDB(t)
	plugin := saveExamplePlugin(t, "")
	user := newUser(t, "poll-plugin", 1, 1000000)
	token := mustToken(t, user.ID, 1000000)
	var mode atomic.Int32
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch mode.Load() {
		case 0:
			w.WriteHeader(429)
			_, _ = io.WriteString(w, `{}`)
		case 1:
			_, _ = io.WriteString(w, `{"status":"unknown"}`)
		default:
			_, _ = io.WriteString(w, `{"status":"failed"}`)
		}
	}))
	defer server.Close()
	ch := &model.Channel{Name: "poll", Type: channel.ChannelTypeTaskPlugin, TaskPluginID: plugin.ID, Key: "key", BaseURL: server.URL, Models: "sora-2", Group: "default", Status: 1, Weight: 1}
	if err := model.InsertChannel(ch); err != nil {
		t.Fatal(err)
	}
	pre, err := relay.PreConsumeQuotaAmount(user.ID, token.ID, false, 80000)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ := json.Marshal(relay.PluginTaskPricingSnapshot{PriceData: *relay.NewPriceData("sora-2", "default"), Usage: taskplugin.Usage{Seconds: 8, N: 2}})
	task := &model.Task{TaskID: "id", TaskPluginID: plugin.ID, ChannelID: ch.ID, UserID: user.ID, TokenID: token.ID, Model: "sora-2", Quota: pre, RequestMeta: `{"model":"sora-2","seconds":8,"n":2}`, PricingMeta: string(snapshot)}
	if err := model.InsertTask(task); err != nil {
		t.Fatal(err)
	}
	for _, m := range []int32{0, 1} {
		mode.Store(m)
		if _, err := service.PollTask(context.Background(), task); err == nil {
			t.Fatal("expected transient error")
		}
		stored, _ := model.GetTaskByID(task.ID)
		if stored.Status != "submitted" || stored.PendingStatus != "" {
			t.Fatal("transient error finalized task")
		}
	}
	mode.Store(2)
	const callback = "plugin:fail-log"
	if err := model.DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "Log" {
			tx.AddError(errors.New("injected log failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = model.DB.Callback().Create().Remove(callback) }()
	if _, err := service.PollTask(context.Background(), task); err == nil {
		t.Fatal("expected settlement failure")
	}
	stored, _ := model.GetTaskByID(task.ID)
	if stored.Status != "submitted" || stored.PendingStatus != "failed" {
		t.Fatalf("lost settlement intent: %+v", stored)
	}
	_ = model.DB.Callback().Create().Remove(callback)
	if _, err := service.PollTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	after, _ := model.GetUserByID(user.ID)
	if after.Quota != user.Quota || calls.Load() != 3 {
		t.Fatal("refund recovery failed or repolled upstream")
	}
}

func TestPluginVersionImmutableAndMigration(t *testing.T) {
	newTestDB(t)
	p := saveExamplePlugin(t, "")
	if _, err := model.SaveTaskPlugin(p.Source+"\n", taskplugin.Meta{Key: p.Key, Version: p.Version, APIVersion: 1}); err == nil {
		t.Fatal("version overwritten")
	}
	if err := model.SetTaskPluginEnabled(p.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := relay.LoadTaskPlugin(context.Background(), p.ID, true); err == nil {
		t.Fatal("disabled version accepted new task")
	}
	if _, err := relay.LoadTaskPlugin(context.Background(), p.ID, false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.Exec("CREATE TABLE tasks (id integer PRIMARY KEY, task_id text)").Error; err != nil {
		t.Fatal(err)
	}
	if err := legacy.Exec("INSERT INTO tasks (id, task_id) VALUES (1, 'legacy-job')").Error; err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := legacy.DB()
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	if err := model.CloseDB(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := model.InitDB(path); err != nil {
			t.Fatal(err)
		}
		old, err := model.GetTaskByID(1)
		if err != nil || old.TaskID != "legacy-job" || old.TaskPluginID != 0 {
			t.Fatalf("legacy migration: %+v %v", old, err)
		}
		if err := model.CloseDB(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPluginSubmissionProtectsOnlyItsChannel(t *testing.T) {
	newTestDB(t)
	if err := setting.UpdatePricing(setting.OptionKeyModelPrice, `{"sora-2":0.01}`); err != nil {
		t.Fatal(err)
	}
	plugin := saveExamplePlugin(t, "")
	arrived, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	releaseRequest := func() { once.Do(func() { close(release) }) }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(arrived)
		<-release
		_, _ = io.WriteString(w, `{"id":"inflight"}`)
	}))
	defer server.Close()
	defer releaseRequest()
	ch := &model.Channel{Name: "inflight", Type: channel.ChannelTypeTaskPlugin, TaskPluginID: plugin.ID, Key: "key", BaseURL: server.URL, Models: "sora-2", Group: "default", Status: 1, Weight: 1}
	if err := model.InsertChannel(ch); err != nil {
		t.Fatal(err)
	}
	other := &model.Channel{Name: "unrelated", Type: channel.ChannelTypeOpenAI, Key: "other", BaseURL: server.URL, Models: "gpt-test", Group: "default", Status: 1, Weight: 1}
	if err := model.InsertChannel(other); err != nil {
		t.Fatal(err)
	}
	user := newUser(t, "inflight", 1, 1000000)
	token := mustToken(t, user.ID, 1000000)
	result := make(chan *httptest.ResponseRecorder, 1)
	go func() { result <- submitExamplePlugin(t, token) }()
	select {
	case <-arrived:
	case <-time.After(5 * time.Second):
		t.Fatal("submission never reached upstream")
	}
	var admitted model.Task
	if err := model.DB.First(&admitted, "channel_id = ?", ch.ID).Error; err != nil {
		t.Fatal(err)
	}
	if admitted.Status != model.TaskStatusSubmitting || admitted.Quota != 80000 {
		t.Fatalf("missing durable admission: %+v", admitted)
	}
	update := make(chan error, 1)
	changed := *ch
	changed.BaseURL = "https://other.test"
	go func() { update <- model.UpdateChannel(&changed) }()
	select {
	case err := <-update:
		if !errors.Is(err, model.ErrTaskPluginChannelBusy) {
			t.Fatalf("credential change allowed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("channel update waited for upstream HTTP")
	}
	other.Name = "updated"
	go func() { update <- model.UpdateChannel(other) }()
	select {
	case err := <-update:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("unrelated channel update blocked")
	}
	go func() { update <- model.DeleteChannel(other.ID) }()
	select {
	case err := <-update:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("unrelated channel deletion blocked")
	}
	releaseRequest()
	if response := <-result; response.Code != 200 {
		t.Fatalf("submit failed: %s", response.Body)
	}
	stored, err := model.GetTaskByID(admitted.ID)
	if err != nil || stored.Status != "submitted" || stored.TaskID != "inflight" {
		t.Fatalf("submission completion failed: %+v %v", stored, err)
	}
}

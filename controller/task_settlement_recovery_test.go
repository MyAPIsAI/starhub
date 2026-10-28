package controller

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"starhub/common"
	"starhub/model"
	"starhub/relay"
	"starhub/relay/channel"
	"starhub/service"
	"starhub/setting"
	"sync/atomic"
	"testing"
)

func TestTaskSettlementRecoversDurableIntent(t *testing.T) {
	for _, tc := range []struct {
		name, status string
		actual       int64
		logStatus    int
		expression   string
	}{
		{"success", "completed", 80000, 1, ""}, {"failure", "failed", 0, 2, ""},
		{"cancelled", "cancelled", 40000, 3, ""},
		{"charged failure", "failed", 20000, 2, `{"sora-2":"0.04"}`},
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
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"recover","status":"` + tc.status + `","progress":100}`))
			}))
			defer upstream.Close()
			ch := &model.Channel{Name: "recovery", Type: channel.ChannelTypeOpenAI, Key: "review", BaseURL: upstream.URL, Models: "sora-2", Group: "default", Status: common.ChannelStatusEnabled}
			if err := model.InsertChannel(ch); err != nil {
				t.Fatal(err)
			}
			user := newUser(t, "recover", common.RoleUser, 1000000)
			token := mustToken(t, user.ID, 1000000)
			pre, err := relay.PreConsumeQuotaAmount(user.ID, token.ID, false, 80000)
			if err != nil {
				t.Fatal(err)
			}
			task := &model.Task{TaskID: "recover", Platform: "sora", Action: "video", Model: "sora-2", RequestedModel: "veyra-auto", Protocol: "OpenAI", UserID: user.ID, TokenID: token.ID, ChannelID: ch.ID, Group: "default", UserGroup: "default", Quota: pre, RequestMeta: `{"model":"sora-2","prompt":"review","seconds":8,"n":2}`}
			if err := model.InsertTask(task); err != nil {
				t.Fatal(err)
			}
			callback := "review:fail_settlement_log"
			if err := model.DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "Log" {
					tx.AddError(errors.New("injected log failure"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			defer model.DB.Callback().Create().Remove(callback)
			if _, err := service.PollTask(context.Background(), task); err == nil {
				t.Fatal("expected injected settlement failure")
			}
			stored, err := model.GetTaskByID(task.ID)
			if err != nil {
				t.Fatal(err)
			}
			if stored.Status == stored.PendingStatus || stored.PendingStatus == "" || stored.PendingQuota != tc.actual {
				t.Fatalf("intent or terminal rollback lost: %+v", stored)
			}
			beforeUser, _ := model.GetUserByID(user.ID)
			beforeToken, _ := model.GetTokenByID(token.ID)
			if beforeUser.Quota != user.Quota-pre || beforeUser.UsedQuota != 0 || beforeToken.RemainQuota != token.RemainQuota-pre || beforeToken.UsedQuota != 0 {
				t.Fatal("failed transaction partially moved funds")
			}
			if count, _ := model.CountLogs(user.ID); count != 0 {
				t.Fatal("failed transaction retained a log")
			}
			if err := model.DB.Callback().Create().Remove(callback); err != nil {
				t.Fatal(err)
			}
			upstream.Close()
			// Simulate a restart with upstream output unavailable and pricing changed.
			if err := setting.UpdatePricing(setting.OptionKeyModelPrice, `{"sora-2":100}`); err != nil {
				t.Fatal(err)
			}
			service.PollPendingTasksOnce(context.Background())
			stored, err = model.GetTaskByID(task.ID)
			if err != nil || stored.PendingStatus != "" || stored.Status != task.PendingStatus {
				t.Fatalf("recovery failed: %+v %v", stored, err)
			}
			logs, total, err := model.ListLogs(user.ID, 1, 10, "")
			if err != nil || total != 1 || logs[0].Quota != tc.actual || logs[0].Status != tc.logStatus || logs[0].RequestedModel != "veyra-auto" || logs[0].Protocol != "OpenAI" {
				t.Fatalf("recovered log incorrect: %+v %v", logs, err)
			}
			afterUser, _ := model.GetUserByID(user.ID)
			afterToken, _ := model.GetTokenByID(token.ID)
			afterChannel, _ := model.GetChannelByID(ch.ID)
			if afterUser.Quota != user.Quota-tc.actual || afterUser.UsedQuota != tc.actual || afterToken.RemainQuota != token.RemainQuota-tc.actual || afterToken.UsedQuota != tc.actual || afterChannel.UsedQuota != tc.actual {
				t.Fatal("recovery charged incorrectly")
			}
			service.PollPendingTasksOnce(context.Background())
			if count, _ := model.CountLogs(user.ID); count != 1 || calls.Load() != 1 {
				t.Fatal("recovery duplicated settlement or repolled expired output")
			}
		})
	}
}

func TestTaskSettlementRefundsAfterTokenDeletion(t *testing.T) {
	newTestDB(t)
	user := newUser(t, "deleted-task-key", common.RoleUser, 1000000)
	token := mustToken(t, user.ID, 1000000)
	pre, err := relay.PreConsumeQuotaAmount(user.ID, token.ID, false, 80000)
	if err != nil {
		t.Fatal(err)
	}
	task := &model.Task{TaskID: "deleted-key", Model: "sora-2", UserID: user.ID, TokenID: token.ID, Quota: pre}
	if err := model.InsertTask(task); err != nil {
		t.Fatal(err)
	}
	if err := model.DeleteToken(token.ID); err != nil {
		t.Fatal(err)
	}
	stored, err := model.RecordTaskSettlement(task.ID, "cancelled", "review", "", 40000)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := relay.FinalizeTaskQuota(stored)
	if err != nil || !claimed {
		t.Fatalf("deleted key prevented settlement: %v %v", claimed, err)
	}
	after, _ := model.GetUserByID(user.ID)
	if after.Quota != 960000 || after.UsedQuota != 40000 {
		t.Fatalf("deleted key lost refund: %+v", after)
	}
	if claimed, err := relay.FinalizeTaskQuota(stored); err != nil || claimed {
		t.Fatal("deleted key settlement repeated")
	}
}

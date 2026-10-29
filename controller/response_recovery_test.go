package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"starhub/model"
	"starhub/service"
	"starhub/setting"
	"sync/atomic"
	"testing"
)

func TestGatewayConversationReferencesAreOwnedAndAdvanceAfterSuccessfulTurns(t *testing.T) {
	newTestDB(t)
	var calls atomic.Int64
	var previous string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		raw, _ := io.ReadAll(r.Body)
		var input map[string]json.RawMessage
		json.Unmarshal(raw, &input)
		if _, ok := input["conversation"]; ok {
			t.Error("gateway conversation ID leaked upstream")
		}
		if calls.Load() == 2 {
			if json.Unmarshal(input["previous_response_id"], &previous) != nil || previous != "resp_conv_1" {
				t.Error("conversation did not retain original response context")
			}
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"resp_conv_%d","model":"gpt-4o","status":"completed","output":[],"usage":{"input_tokens":10,"output_tokens":2}}`, calls.Load())
	}))
	defer up.Close()
	responsesTestChannel(t, up.URL, 1)
	u := newUser(t, "conv-owner", 1, 1000000)
	token := mustToken(t, u.ID, 1000000)
	conversation, err := model.CreateResponseConversation(u.ID, token.ID, map[string]string{"kind": "local"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		responsesTestHandler().ServeHTTP(rec, responsesTestRequest(token.Key, fmt.Sprintf(`{"model":"gpt-4o","conversation":%q,"input":"next"}`, conversation.ID)))
		if rec.Code != 200 {
			t.Fatalf("conversation turn: %d %s", rec.Code, rec.Body)
		}
	}
	stored, err := model.GetResponseConversation(conversation.ID, u.ID, token.ID)
	if err != nil || stored.Busy || stored.LastResponseID != "resp_conv_2" {
		t.Fatal("conversation was not durably advanced")
	}
	other := mustToken(t, u.ID, 1000000)
	rec := httptest.NewRecorder()
	responsesTestHandler().ServeHTTP(rec, responsesTestRequest(other.Key, fmt.Sprintf(`{"model":"gpt-4o","conversation":%q,"input":"foreign"}`, conversation.ID)))
	if rec.Code != 404 || calls.Load() != 2 {
		t.Fatal("another API key reused private conversation")
	}
}
func TestBackgroundResponsePendingSettlementRecoversAfterRestartWithoutRepoll(t *testing.T) {
	newTestDB(t)
	var gets atomic.Int64
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" {
			fmt.Fprint(w, `{"id":"resp_restart","status":"queued","store":true}`)
			return
		}
		gets.Add(1)
		fmt.Fprint(w, `{"id":"resp_restart","status":"completed","output":[],"usage":{"input_tokens":100,"output_tokens":20}}`)
	}))
	defer up.Close()
	responsesTestChannel(t, up.URL, 1)
	u := newUser(t, "bg-restart", 1, 1000000)
	token := mustToken(t, u.ID, 1000000)
	rec := httptest.NewRecorder()
	responsesTestHandler().ServeHTTP(rec, responsesTestRequest(token.Key, `{"model":"gpt-4o","input":"hello","background":true,"store":true}`))
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	var task model.Task
	model.DB.First(&task, "platform = ?", "responses")
	if err := model.DB.Exec(`CREATE TRIGGER reject_bg_log BEFORE INSERT ON logs BEGIN SELECT RAISE(ABORT, 'injected'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.PollTask(context.Background(), &task); err == nil {
		t.Fatal("injected billing failure ignored")
	}
	pending, _ := model.GetTaskByID(task.ID)
	if pending.PendingStatus == "" || pending.PendingQuota <= 0 {
		t.Fatal("no durable settlement intent")
	}
	if err := model.DB.Exec("DROP TRIGGER reject_bg_log").Error; err != nil {
		t.Fatal(err)
	}
	var databases []struct{ Name, File string }
	model.DB.Raw("PRAGMA database_list").Scan(&databases)
	var file string
	for _, db := range databases {
		if db.Name == "main" {
			file = db.File
		}
	}
	if file == "" {
		t.Fatal("restart fixture has no durable database")
	}
	if err := model.CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := model.InitDB(file); err != nil {
		t.Fatal(err)
	}
	if err := setting.ApplyOptions(map[string]string{"model_ratio": `{"gpt-4o":1000}`}); err != nil {
		t.Fatal(err)
	}
	fresh, _ := model.GetTaskByID(task.ID)
	if _, err := service.PollTask(context.Background(), fresh); err != nil {
		t.Fatal(err)
	}
	if gets.Load() != 1 {
		t.Fatal("recovery repolled instead of using durable terminal intent")
	}
	logs, total, err := model.ListLogs(u.ID, 1, 10, "")
	if err != nil || total != 1 || logs[0].Quota != pending.PendingQuota {
		t.Fatal("recovery repriced or double-billed")
	}
}

package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"starhub/common"
	"starhub/middleware"
	"starhub/model"
	"starhub/relay/dto"
	"starhub/service"
	"starhub/setting"
	"strings"
	"sync/atomic"
	"testing"
)

const taskAbsoluteRates = `{"sora-2":{"timezone":"UTC","windows":[{"id":"all","label":"All","days":[0,1,2,3,4,5,6],"start":"00:00","end":"24:00","prices":{"input_per_m":"1","output_per_m":"2","cache_read_per_m":"3","cache_write_per_m":"4","audio_input_per_m":"5","audio_output_per_m":"6"}}]}}`
const taskSixClassUsage = `"usage":{"prompt_tokens":400,"completion_tokens":200,"prompt_tokens_details":{"cached_tokens":100,"cache_write_tokens":100,"audio_tokens":100},"completion_tokens_details":{"audio_tokens":100}}`

func TestTaskTokenTariffsNativeAndPluginRecoverWithoutRepricing(t *testing.T) {
	for _, plugin := range []bool{false, true} {
		for _, tc := range []struct {
			status  string
			outside bool
			free    bool
		}{{"completed", false, false}, {"cancelled", false, false}, {"failed", false, false}, {"completed", true, false}, {"completed", false, true}} {
			status := tc.status
			t.Run(fmt.Sprintf("plugin=%v/%s/outside=%v/free=%v", plugin, status, tc.outside, tc.free), func(t *testing.T) {
				newTestDB(t)
				rules := taskAbsoluteRates
				want := int64(1050)
				if tc.outside {
					rules = `{"sora-2":{"timezone":"UTC","windows":[]}}`
					want = 950
				}
				if tc.free {
					rules = strings.NewReplacer(`:"1"`, `:"0"`, `:"2"`, `:"0"`, `:"3"`, `:"0"`, `:"4"`, `:"0"`, `:"5"`, `:"0"`, `:"6"`, `:"0"`).Replace(taskAbsoluteRates)
					want = 0
				}
				if err := setting.ApplyOptions(map[string]string{"model_price": "{}", "model_price_expr": "{}", "model_ratio": `{"sora-2":2}`, "completion_ratio": `{"sora-2":3}`, "cache_ratio": "0.5", "audio_ratio": "2", setting.OptionKeyModelTimeTokenPrices: rules}); err != nil {
					t.Fatal(err)
				}
				var gets, posts atomic.Int64
				up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					if r.Method == "POST" {
						posts.Add(1)
						fmt.Fprint(w, `{"id":"token-task","status":"queued"}`)
						return
					}
					gets.Add(1)
					fmt.Fprintf(w, `{"id":"token-task","status":%q,"progress":100,"video":{"url":"https://example.test/result.mp4"},%s}`, status, taskSixClassUsage)
				}))
				defer up.Close()
				ch := &model.Channel{Name: "token-tariff", Type: 1, Key: "synthetic", BaseURL: up.URL, Models: "sora-2", Group: "default", Status: common.ChannelStatusEnabled, Weight: 1}
				if plugin {
					ch.Type = 61
					ch.TaskPluginID = saveExamplePlugin(t, "").ID
				}
				if err := model.InsertChannel(ch); err != nil {
					t.Fatal(err)
				}
				user := newUser(t, "token-tariff", common.RoleUser, 1000000)
				token := mustToken(t, user.ID, 1000000)
				r := httptest.NewRequest("POST", "/v1/video/generations", strings.NewReader(`{"model":"sora-2","prompt":"fixture","seconds":4,"n":1,"max_output_tokens":1000}`))
				r.Header.Set("Authorization", "Bearer "+token.Key)
				r.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				middleware.TokenAuth(middleware.Distribute(dto.InboundOpenAI)(http.HandlerFunc(RelayTaskSubmitVideo))).ServeHTTP(w, r)
				if w.Code != 200 {
					t.Fatalf("admission: %d %s", w.Code, w.Body.String())
				}
				var task model.Task
				if err := model.DB.First(&task).Error; err != nil {
					t.Fatal(err)
				}
				if task.Quota <= want {
					t.Fatal("reservation did not use the highest unknown token classes")
				}
				// Erase admission rules before the first terminal observation.
				if err := setting.ApplyOptions(map[string]string{setting.OptionKeyModelTimeTokenPrices: "{}", "model_ratio": `{"sora-2":10000}`}); err != nil {
					t.Fatal(err)
				}
				if err := model.DB.Exec(`CREATE TRIGGER reject_task_tariff_log BEFORE INSERT ON logs BEGIN SELECT RAISE(ABORT,'injected'); END`).Error; err != nil {
					t.Fatal(err)
				}
				if _, err := service.PollTask(context.Background(), &task); err == nil {
					t.Fatal("injected settlement failure was ignored")
				}
				pending, _ := model.GetTaskByID(task.ID)
				if pending.PendingQuota != want || pending.PendingStatus == "" {
					t.Fatalf("six captured classes lost: %+v", pending)
				}
				if count, _ := model.CountLogs(user.ID); count != 0 {
					t.Fatal("partial settlement persisted")
				}
				if err := model.DB.Exec("DROP TRIGGER reject_task_tariff_log").Error; err != nil {
					t.Fatal(err)
				}
				var dbs []struct{ Name, File string }
				model.DB.Raw("PRAGMA database_list").Scan(&dbs)
				file := ""
				for _, db := range dbs {
					if db.Name == "main" {
						file = db.File
					}
				}
				if file == "" {
					t.Fatal("restart fixture is not durable")
				}
				if err := model.CloseDB(); err != nil {
					t.Fatal(err)
				}
				if err := model.InitDB(file); err != nil {
					t.Fatal(err)
				}
				for i := 0; i < 2; i++ {
					fresh, _ := model.GetTaskByID(task.ID)
					if _, err := service.PollTask(context.Background(), fresh); err != nil {
						t.Fatal(err)
					}
				}
				logs, total, err := model.ListLogs(user.ID, 1, 10, "")
				if err != nil || total != 1 || logs[0].Quota != want || logs[0].PromptTokens != 400 || logs[0].CompletionTokens != 200 || logs[0].UsageSource != "upstream" || gets.Load() != 1 || posts.Load() != 1 {
					t.Fatalf("recovery duplicated, repriced or lost usage: %+v %v", logs, err)
				}
				afterUser, _ := model.GetUserByID(user.ID)
				afterToken, _ := model.GetTokenByID(token.ID)
				if afterUser.Quota != user.Quota-want || afterToken.RemainQuota != token.RemainQuota-want || afterUser.UsedQuota != want || afterToken.UsedQuota != want {
					t.Fatal("balances did not reconcile")
				}
			})
		}
	}
}

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
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestBackgroundResponsesDurablePollingAndCancelSettleOnce(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(fmt.Sprint(cancel), func(t *testing.T) {
			newTestDB(t)
			var polls atomic.Int64
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == "POST" && r.URL.Path == "/v1/responses" {
					fmt.Fprint(w, `{"id":"resp_bg","model":"gpt-4o","status":"queued","background":true,"store":true}`)
					return
				}
				polls.Add(1)
				status := "completed"
				if strings.HasSuffix(r.URL.Path, "/cancel") {
					status = "cancelled"
				}
				fmt.Fprintf(w, `{"id":"resp_bg","model":"gpt-4o","status":%q,"usage":{"input_tokens":100,"output_tokens":20},"output":[]}`, status)
			}))
			defer up.Close()
			responsesTestChannel(t, up.URL, 1)
			user := newUser(t, "bg-owner", 1, 1000000)
			token := mustToken(t, user.ID, 1000000)
			rec := httptest.NewRecorder()
			responsesTestHandler().ServeHTTP(rec, responsesTestRequest(token.Key, `{"model":"gpt-4o","input":"hello","background":true,"store":true,"max_output_tokens":128}`))
			if rec.Code != 200 {
				t.Fatalf("background: %d %s", rec.Code, rec.Body)
			}
			var tasks []model.Task
			model.DB.Where("platform = ?", "responses").Find(&tasks)
			if len(tasks) != 1 || tasks[0].TaskID != "resp_bg" {
				t.Fatal("background responsibility not persisted")
			}
			logs, _ := model.CountLogs(user.ID)
			if logs != 0 {
				t.Fatal("queued response prematurely settled")
			}
			if cancel {
				req := httptest.NewRequest("POST", "/v1/responses/resp_bg/cancel", nil)
				req.SetPathValue("id", "resp_bg")
				req.Header.Set("Authorization", "Bearer "+token.Key)
				rr := httptest.NewRecorder()
				middleware.TokenResponseControlAuth(http.HandlerFunc(CancelResponse)).ServeHTTP(rr, req)
				if rr.Code != 200 {
					t.Fatalf("cancel: %d %s", rr.Code, rr.Body)
				}
			} else {
				if _, err := service.PollTask(context.Background(), &tasks[0]); err != nil {
					t.Fatal(err)
				}
			}
			fresh, err := model.GetTaskByID(tasks[0].ID)
			if err != nil || !dto.IsTerminalTaskStatus(fresh.Status) {
				t.Fatalf("background not terminal %+v %v", fresh, err)
			}
			if _, err := service.PollTask(context.Background(), fresh); err != nil {
				t.Fatal(err)
			}
			want := 1
			if cancel {
				want = 3
			}
			assertResponsesBill(t, user, token, 100, 20, want)
			if polls.Load() != 1 {
				t.Fatal("settled response polled again")
			}
			ref, err := model.LookupResponseReference(context.Background(), user.ID, token.ID, "resp_bg", time.Now())
			if err != nil || ref.PromptTokens != 100 || ref.CompletionTokens != 20 {
				t.Fatal("reference and accounting did not finalize together")
			}
		})
	}
}
func TestResponsesCompactionAndOwnedContinuation(t *testing.T) {
	newTestDB(t)
	var posts atomic.Int64
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/responses/compact" {
			fmt.Fprint(w, `{"id":"cmp_local","object":"response.compaction","output":[{"type":"compaction","encrypted_content":"opaque"}],"usage":{"input_tokens":10,"output_tokens":2}}`)
			return
		}
		id := "resp_first"
		if posts.Load() > 1 {
			id = "resp_next"
		}
		fmt.Fprintf(w, `{"id":%q,"model":"gpt-4o","status":"completed","output":[],"usage":{"input_tokens":10,"output_tokens":2}}`, id)
	}))
	defer up.Close()
	responsesTestChannel(t, up.URL, 1)
	u := newUser(t, "continuation", common.RoleUser, 1000000)
	token := mustToken(t, u.ID, 1000000)
	for _, body := range []string{`{"model":"gpt-4o","input":"first"}`, `{"model":"gpt-4o","input":"next","previous_response_id":"resp_first"}`} {
		rec := httptest.NewRecorder()
		responsesTestHandler().ServeHTTP(rec, responsesTestRequest(token.Key, body))
		if rec.Code != 200 {
			t.Fatalf("continuation: %d %s", rec.Code, rec.Body)
		}
	}
	req := responsesTestRequest(token.Key, `{"model":"gpt-4o","input":[{"role":"user","content":"hello"}]}`)
	req.URL.Path = "/v1/responses/compact"
	rec := httptest.NewRecorder()
	responsesTestHandler().ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "response.compaction") {
		t.Fatalf("compaction: %d %s", rec.Code, rec.Body)
	}
	logs, _ := model.CountLogs(u.ID)
	if logs != 3 {
		t.Fatal("compaction or continuation billing missing")
	}
}

package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"starhub/common"
	"starhub/model"
	"starhub/service"
)

func TestChannelVideoProbeLifecycle(t *testing.T) {
	newTestDB(t)
	admin := newUser(t, "video-probe-admin", common.RoleAdmin, 1000000)
	const name = "doubao-seedance-2-0-fast-260128"
	submits, polls := 0, 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer video-key" {
			t.Error("wrong submission or polling key")
		}
		switch {
		case r.Method == "POST" && r.URL.Path == "/api/v3/contents/generations/tasks":
			submits++
			var count int64
			model.DB.Model(&model.Task{}).Where("platform = ? AND status = ?", model.ArkVideoProbePlatform, model.TaskStatusSubmitting).Count(&count)
			if count != 1 {
				t.Error("probe must be durable before the upstream call")
			}
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["duration"] != float64(4) || body["resolution"] != "480p" || body["model"] != name {
				t.Error("probe was not bounded to a small video")
			}
			fmt.Fprint(w, `{"id":"probe-upstream-task"}`)
		case r.Method == "GET" && r.URL.Path == "/api/v3/contents/generations/tasks/probe-upstream-task":
			polls++
			if polls == 1 {
				fmt.Fprint(w, `{"status":"running"}`)
			} else {
				fmt.Fprint(w, `{"status":"succeeded","duration":4,"content":{"video_url":"https://example.test/video.mp4"},"usage":{"completion_tokens":100,"total_tokens":100}}`)
			}
		default:
			t.Errorf("unexpected probe request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer upstream.Close()
	ch := &model.Channel{Type: 32, Name: "video-probe", BaseURL: upstream.URL, Key: "disabled-key\nvideo-key", MultiKeyStatus: "2,1", Models: name, Group: "default", Status: 1}
	if err := model.InsertChannel(ch); err != nil {
		t.Fatal(err)
	}
	submit := func(requestID, prompt string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]any{"model": name, "prompt": prompt, "request_id": requestID, "confirm_upstream_charge": true})
		return call(SubmitChannelVideoTest, testRequest{method: "POST", target: fmt.Sprintf("/api/channel/video-test/%d/task", ch.ID), path: map[string]string{"id": fmt.Sprint(ch.ID)}, body: string(body), userID: admin.ID, role: common.RoleAdmin})
	}
	requestID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	unconfirmed := testRequest{method: "POST", target: fmt.Sprintf("/api/channel/video-test/%d/task", ch.ID), path: map[string]string{"id": fmt.Sprint(ch.ID)}, body: `{"model":"` + name + `","prompt":"A ball","request_id":"` + requestID + `","confirm_upstream_charge":false}`, userID: admin.ID, role: common.RoleAdmin}
	expectError(t, call(SubmitChannelVideoTest, unconfirmed), 400, "video_probe_confirmation_required")
	if submits != 0 {
		t.Fatal("unconfirmed video probe reached upstream")
	}
	var first, again struct {
		ID       int64  `json:"id"`
		Status   string `json:"status"`
		VideoURL string `json:"video_url"`
	}
	decodeData(t, submit(requestID, "A rolling red ball"), &first)
	decodeData(t, submit(requestID, "A rolling red ball"), &again)
	if first.ID != again.ID || first.Status != "submitted" || submits != 1 {
		t.Fatal("duplicate paid submission")
	}
	expectError(t, submit(requestID, "Different prompt"), 400, "video_probe_request_conflict")
	expectError(t, submit("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", "A rolling red ball"), 400, "video_probe_already_running")
	ch.Key = "replacement-key"
	if err := model.UpdateChannel(ch); err == nil {
		t.Fatal("pending probe did not protect its credential")
	}
	ch.Key = "disabled-key\nvideo-key"
	task, err := model.GetTaskByID(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if task.KeyIndex != 1 || task.Quota != 0 {
		t.Fatal("probe used the wrong account or reserved customer quota")
	}
	task, err = service.PollTask(t.Context(), task)
	if err != nil || task.Status != "in_progress" {
		t.Fatalf("background probe polling failed: %v", err)
	}
	req := testRequest{method: "GET", target: fmt.Sprintf("/api/channel/video-test/%d/task", ch.ID), path: map[string]string{"id": fmt.Sprint(ch.ID)}, userID: admin.ID, role: common.RoleAdmin}
	decodeData(t, call(GetChannelVideoTest, req), &again)
	if again.Status != "succeeded" || again.VideoURL != "https://example.test/video.mp4" || polls != 2 {
		t.Fatalf("video result was not restored: %+v", again)
	}
	decodeData(t, call(GetChannelVideoTest, req), &again)
	if polls != 2 {
		t.Fatal("terminal probes must not refetch signed upstream results")
	}
	after, _ := model.GetUserByID(admin.ID)
	saved, _ := model.GetChannelByID(ch.ID)
	if after.Quota != admin.Quota || saved.UsedQuota != 0 || saved.Status != 1 || saved.MultiKeyStatus != "2,1" {
		t.Fatal("admin generation changed customer quota or channel health")
	}
	var logs int64
	model.DB.Model(&model.Log{}).Count(&logs)
	if logs != 0 {
		t.Fatal("admin probe entered customer settlement")
	}
}

func TestChannelVideoProbeRejectsAuthenticationAndUncertainRetries(t *testing.T) {
	for _, uncertain := range []bool{false, true} {
		t.Run(fmt.Sprint(uncertain), func(t *testing.T) {
			newTestDB(t)
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if uncertain {
					conn, _, _ := w.(http.Hijacker).Hijack()
					_ = conn.Close()
					return
				}
				w.WriteHeader(401)
				fmt.Fprint(w, `{"error":{"code":"AuthenticationError","message":"video-secret"}}`)
			}))
			defer upstream.Close()
			ch := mustChannel(t, "probe-auth", "doubao-seedance-2-0-fast-260128", "video-secret", 1)
			ch.Type = 32
			ch.BaseURL = upstream.URL
			if err := model.UpdateChannel(ch); err != nil {
				t.Fatal(err)
			}
			body := `{"model":"doubao-seedance-2-0-fast-260128","prompt":"A ball","request_id":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa","confirm_upstream_charge":true}`
			req := testRequest{method: "POST", target: fmt.Sprintf("/api/channel/video-test/%d/task", ch.ID), path: map[string]string{"id": fmt.Sprint(ch.ID)}, body: body, userID: 1, role: common.RoleAdmin}
			for range 2 {
				response := call(SubmitChannelVideoTest, req)
				var result struct {
					Status  string `json:"status"`
					Message string `json:"message"`
				}
				decodeData(t, response, &result)
				want := "upstream_video_authentication_failed"
				if uncertain {
					want = "video_probe_submission_uncertain"
				}
				if result.Status != "failed" || result.Message != want || strings.Contains(response.Body.String(), "video-secret") {
					t.Fatal("unsafe or misleading video failure")
				}
			}
			if calls != 1 {
				t.Fatal("a failed or uncertain submission was retried")
			}
		})
	}
}

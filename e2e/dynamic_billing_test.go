package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"starhub/common"
	"starhub/model"
)

func TestDynamicBillingImage(t *testing.T) {
	cases := []struct {
		name      string
		stream    bool
		requested int
		delivered int
	}{
		{name: "json_under_delivery", requested: 3, delivered: 2},
		{name: "stream_over_delivery", stream: true, requested: 2, delivered: 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			arrived := make(chan struct{})
			release := make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			t.Cleanup(unblock)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/images/generations" {
					http.NotFound(w, r)
					return
				}
				close(arrived)
				<-release
				if tc.stream {
					w.Header().Set("Content-Type", "text/event-stream")
					for i := 0; i < tc.delivered; i++ {
						fmt.Fprintf(w, "event: image_generation.completed\ndata: {\"type\":\"image_generation.completed\",\"output_index\":%d,\"b64_json\":\"QUJD\"}\n\n", i)
					}
					fmt.Fprint(w, "data: [DONE]\n\n")
					return
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"data":[`)
				for i := 0; i < tc.delivered; i++ {
					if i > 0 {
						fmt.Fprint(w, ",")
					}
					fmt.Fprintf(w, `{"url":"https://cdn.example/image-%d.png"}`, i)
				}
				fmt.Fprint(w, `]}`)
			}))
			t.Cleanup(upstream.Close)

			env := setupRelayApp(t)
			createChannelID(t, env, map[string]any{
				"name": "dynamic-image", "key": upstreamKey, "base_url": upstream.URL,
				"models": "gpt-image-1",
			})
			setOption(t, env, "model_price", `{"gpt-image-1":0.99}`)
			setOption(t, env, "model_price_expr", `{"gpt-image-1":"0.04 * count * (size == 1792 ? 2 : 1) * quality"}`)

			result := make(chan apiResult, 1)
			go func() {
				result <- doRequest(env.base, http.MethodPost, "/v1/images/generations", env.apiKey, map[string]any{
					"model": "gpt-image-1", "prompt": "a cat", "n": tc.requested,
					"size": "1024x1792", "quality": "high", "stream": tc.stream,
				})
			}()
			select {
			case <-arrived:
			case <-time.After(10 * time.Second):
				unblock()
				t.Fatal("image request did not reach upstream")
			}

			unitQuota := int64(60000) // $0.04 * size 2 * quality 1.5 * 500,000.
			wantPre := unitQuota * int64(tc.requested)
			assertDynamicTokenQuota(t, env.tokenID, common.DefaultTokenQuota-wantPre)
			unblock()
			select {
			case res := <-result:
				if res.Status != http.StatusOK {
					t.Fatalf("image response: %d %s", res.Status, res.Body)
				}
				if tc.stream && strings.Count(res.Body, "event: image_generation.completed") != tc.delivered {
					t.Fatalf("completed image events: %s", res.Body)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("image request did not finish")
			}
			wantActual := unitQuota * int64(tc.delivered)
			assertDynamicTokenQuota(t, env.tokenID, common.DefaultTokenQuota-wantActual)
			assertDynamicLogQuota(t, env.rootID, "gpt-image-1", wantActual)
		})
	}
}

func TestDynamicBillingVideo(t *testing.T) {
	cases := []struct {
		name       string
		upstream   string
		status     string
		wantActual int64
	}{
		{name: "success", upstream: "completed", status: "succeeded", wantActual: 320000},
		{name: "cancelled", upstream: "cancelled", status: "cancelled", wantActual: 160000},
		{name: "failed", upstream: "failed", status: "failed", wantActual: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var polls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer "+upstreamKey {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodPost && r.URL.Path == "/v1/videos":
					fmt.Fprint(w, `{"id":"dynamic-video","status":"queued"}`)
				case r.Method == http.MethodGet && r.URL.Path == "/v1/videos/dynamic-video":
					if polls.Add(1) == 1 {
						fmt.Fprint(w, `{"id":"dynamic-video","status":"in_progress","progress":25}`)
					} else {
						fmt.Fprintf(w, `{"id":"dynamic-video","status":%q}`, tc.upstream)
					}
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(upstream.Close)

			env := setupRelayApp(t)
			createChannelID(t, env, map[string]any{
				"name": "dynamic-video", "key": upstreamKey, "base_url": upstream.URL,
				"models": "sora-2",
			})
			setOption(t, env, "model_price", `{"sora-2":0.01}`)
			setOption(t, env, "model_price_expr", `{"sora-2":"seconds * 0.04 * n * task_status"}`)
			res := doRequest(env.base, http.MethodPost, "/v1/video/generations", env.apiKey, map[string]any{
				"model": "sora-2", "prompt": "a cat", "seconds": 8, "n": 2,
			})
			if res.Status != http.StatusOK {
				t.Fatalf("video submit: %d %s", res.Status, res.Body)
			}
			var submit struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal([]byte(res.Body), &submit); err != nil || submit.ID == "" {
				t.Fatalf("video submit ID: %s (%v)", res.Body, err)
			}
			const wantPre int64 = 320000 // 8 seconds * $0.04 * 2 videos * 500,000.
			assertDynamicTokenQuota(t, env.tokenID, common.DefaultTokenQuota-wantPre)
			setOption(t, env, "model_price_expr", `{"sora-2":"0.001"}`)

			path := "/v1/video/generations/" + submit.ID
			res = doRequest(env.base, http.MethodGet, path, env.apiKey, nil)
			if res.Status != http.StatusOK || !strings.Contains(res.Body, `"status":"in_progress"`) {
				t.Fatalf("video progress: %d %s", res.Status, res.Body)
			}
			assertDynamicTokenQuota(t, env.tokenID, common.DefaultTokenQuota-wantPre)
			res = doRequest(env.base, http.MethodGet, path, env.apiKey, nil)
			if res.Status != http.StatusOK || !strings.Contains(res.Body, `"status":"`+tc.status+`"`) {
				t.Fatalf("video terminal state: %d %s", res.Status, res.Body)
			}
			assertDynamicTokenQuota(t, env.tokenID, common.DefaultTokenQuota-tc.wantActual)
			assertDynamicLogQuota(t, env.rootID, "sora-2", tc.wantActual)
		})
	}
}

func assertDynamicTokenQuota(t *testing.T, tokenID, want int64) {
	t.Helper()
	token, err := model.GetTokenByID(tokenID)
	if err != nil {
		t.Fatalf("read token: %v", err)
	}
	if token.RemainQuota != want {
		t.Fatalf("token remaining quota = %d, want %d", token.RemainQuota, want)
	}
}

func assertDynamicLogQuota(t *testing.T, userID int64, modelName string, want int64) {
	t.Helper()
	logs, total, err := model.ListLogs(userID, 1, 10, modelName)
	if err != nil || total != 1 || len(logs) != 1 {
		t.Fatalf("consumption logs: total=%d entries=%d err=%v", total, len(logs), err)
	}
	if logs[0].Quota != want {
		t.Fatalf("logged quota = %d, want %d", logs[0].Quota, want)
	}
}

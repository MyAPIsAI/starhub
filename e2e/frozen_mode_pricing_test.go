package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"starhub/common"
	"starhub/model"
	"starhub/relay"
	"starhub/relay/channel"
)

func TestInFlightNonChatRequestsRetainCapturedPrices(t *testing.T) {
	for _, mode := range []string{"embeddings", "rerank", "tts", "stt", "image-json", "image-stream", "image-edit"} {
		t.Run(mode, func(t *testing.T) {
			env := setupRelayApp(t)
			update := func(version int64, price string) {
				t.Helper()
				response := doRequest(env.base, "PUT", "/api/option/pricing", env.session, map[string]any{"expected_version": version, "options": []model.Option{{Key: "model_price", Value: `{"gpt-4o":` + price + `}`}}})
				if response.Status != 200 {
					t.Fatalf("price update: %d %s", response.Status, response.Body)
				}
			}
			update(0, "0.01")
			started, release := make(chan struct{}, 1), make(chan struct{})
			var releaseOnce sync.Once
			response := `{"object":"list","data":[{"embedding":[0.1],"index":0}],"usage":{"prompt_tokens":100,"total_tokens":100}}`
			path := "/v1/embeddings"
			payload := map[string]any{"model": "gpt-4o", "input": "hello"}
			contentType := "application/json"
			switch mode {
			case "rerank":
				path = "/v1/rerank"
				payload = map[string]any{"model": "gpt-4o", "query": "hello", "documents": []string{"world"}}
				response = `{"results":[{"index":0,"relevance_score":0.9}],"usage":{"total_tokens":100}}`
			case "tts":
				path = "/v1/audio/speech"
				payload = map[string]any{"model": "gpt-4o", "input": "hello", "voice": "alloy"}
				response = "synthetic-local-audio"
			case "stt":
				path = "/v1/audio/transcriptions"
				response = `{"text":"hello"}`
			case "image-json", "image-stream", "image-edit":
				path = "/v1/images/generations"
				payload = map[string]any{"model": "gpt-4o", "prompt": "a shape", "size": "1024x1024", "n": 2, "stream": mode == "image-stream"}
				response = `{"data":[{"b64_json":"QUJD"},{"b64_json":"REVG"}]}`
				if mode == "image-edit" {
					path = "/v1/images/edits"
				}
				if mode == "image-stream" {
					response = "event: image_generation.completed\ndata: {\"type\":\"image_generation.completed\",\"output_index\":0,\"b64_json\":\"QUJD\"}\n\nevent: image_generation.completed\ndata: {\"type\":\"image_generation.completed\",\"output_index\":1,\"b64_json\":\"REVG\"}\n\ndata: [DONE]\n\n"
				}
			}
			body, _ := json.Marshal(payload)
			if mode == "stt" || mode == "image-edit" {
				var buffer bytes.Buffer
				form := multipart.NewWriter(&buffer)
				_ = form.WriteField("model", "gpt-4o")
				field, file := "file", "fixture.wav"
				if mode == "image-edit" {
					field, file = "image", "fixture.png"
					_ = form.WriteField("prompt", "a shape")
					_ = form.WriteField("size", "1024x1024")
					_ = form.WriteField("n", "2")
				}
				part, err := form.CreateFormFile(field, file)
				if err != nil {
					t.Fatal(err)
				}
				_, _ = part.Write([]byte("synthetic-local-file"))
				_ = form.Close()
				body = buffer.Bytes()
				contentType = form.FormDataContentType()
			}
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				select {
				case started <- struct{}{}:
				default:
				}
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
				kind := "application/json"
				if mode == "tts" {
					kind = "audio/wav"
				}
				if mode == "image-stream" {
					kind = "text/event-stream"
				}
				w.Header().Set("Content-Type", kind)
				_, _ = io.WriteString(w, response)
			}))
			defer upstream.Close()
			defer releaseOnce.Do(func() { close(release) })
			if err := model.InsertChannel(&model.Channel{Name: "Frozen non-chat fixture", Type: channel.ChannelTypeOpenAI, Key: "synthetic-local", BaseURL: upstream.URL, Models: "gpt-4o", Group: "default", Status: common.ChannelStatusEnabled}); err != nil {
				t.Fatal(err)
			}
			call := func() apiResult {
				request, err := http.NewRequest(http.MethodPost, env.base+path, bytes.NewReader(body))
				if err != nil {
					return apiResult{Status: -1, Body: err.Error()}
				}
				request.Header.Set("Authorization", "Bearer "+env.apiKey)
				request.Header.Set("Content-Type", contentType)
				result, err := http.DefaultClient.Do(request)
				if err != nil {
					return apiResult{Status: -1, Body: err.Error()}
				}
				defer result.Body.Close()
				raw, _ := io.ReadAll(result.Body)
				return apiResult{Status: result.StatusCode, Body: string(raw)}
			}
			done := make(chan apiResult, 1)
			go func() { done <- call() }()
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("request did not reach local upstream")
			}
			update(1, "0.2")
			releaseOnce.Do(func() { close(release) })
			select {
			case result := <-done:
				if result.Status != 200 {
					t.Fatalf("in-flight mode: %d %s", result.Status, result.Body)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("mode request did not finish")
			}
			original, current := int64(5000), int64(100000)
			if strings.HasPrefix(mode, "image") {
				original *= 2
				current *= 2
			}
			logs, count, err := model.ListLogs(env.rootID, 1, 10, "")
			if err != nil || count != 1 || logs[0].Quota != original {
				t.Fatalf("mode was repriced: %+v %v", logs, err)
			}
			if result := call(); result.Status != 200 {
				t.Fatalf("next mode request: %d %s", result.Status, result.Body)
			}
			logs, count, err = model.ListLogs(env.rootID, 1, 10, "")
			if err != nil || count != 2 || logs[0].Quota != current || logs[1].Quota != original {
				t.Fatalf("next mode did not use new price: %+v %v", logs, err)
			}
			user, err := model.GetUserByID(env.rootID)
			if err != nil || user.Quota != 100_000_000-original-current || user.UsedQuota != original+current {
				t.Fatal("mode reservation/refund did not reconcile")
			}
		})
	}
}

func TestVideoSubmitKeepsCapturedPricesWhileUpstreamAccepts(t *testing.T) {
	env := setupRelayApp(t)
	setOption(t, env, "model_price", `{"sora-2":0.01}`)
	started, release := make(chan struct{}, 1), make(chan struct{})
	var releaseOnce sync.Once
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			select {
			case started <- struct{}{}:
			default:
			}
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
			fmt.Fprint(w, `{"id":"frozen_video","status":"queued","progress":0}`)
			return
		}
		fmt.Fprint(w, `{"id":"frozen_video","status":"completed","progress":100,"video":{"url":"https://example.invalid/local-fixture.mp4"}}`)
	}))
	defer upstream.Close()
	defer releaseOnce.Do(func() { close(release) })
	createChannelID(t, env, map[string]any{"name": "Frozen video", "key": upstreamKey, "base_url": upstream.URL, "models": "sora-2"})
	done := make(chan apiResult, 1)
	go func() {
		done <- doRequest(env.base, "POST", "/v1/video/generations", env.apiKey, map[string]any{"model": "sora-2", "prompt": "a shape", "seconds": 8, "n": 2})
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("video submission did not reach local upstream")
	}
	setOption(t, env, "model_price", `{"sora-2":0.02}`)
	setOption(t, env, "group_ratio", `{"default":2}`)
	releaseOnce.Do(func() { close(release) })
	var id string
	select {
	case result := <-done:
		if result.Status != 200 {
			t.Fatalf("submit: %d %s", result.Status, result.Body)
		}
		var submit struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal([]byte(result.Body), &submit); err != nil || submit.ID == "" {
			t.Fatal("missing task ID")
		}
		id = submit.ID
	case <-time.After(5 * time.Second):
		t.Fatal("video submit did not finish")
	}
	result := doRequest(env.base, "GET", "/v1/video/generations/"+id, env.apiKey, nil)
	if result.Status != 200 || !strings.Contains(result.Body, `"status":"succeeded"`) {
		t.Fatalf("poll: %d %s", result.Status, result.Body)
	}
	logs, count, err := model.ListLogs(env.rootID, 1, 10, "sora-2")
	if err != nil || count != 1 || logs[0].Quota != 80000 {
		t.Fatalf("pending task was repriced: %+v %v", logs, err)
	}
	user, err := model.GetUserByID(env.rootID)
	if err != nil || user.Quota != 100_000_000-80000 {
		t.Fatal("task reservation and settlement disagree")
	}
	tasks, err := model.ListPendingTasks(10)
	if err != nil || len(tasks) != 0 {
		t.Fatal("completed task remained pending")
	}
}

func TestRealtimeSessionKeepsCapturedPricesUntilSettlement(t *testing.T) {
	env := setupRelayApp(t)
	setOption(t, env, "model_price", `{"gpt-4o":0.01}`)
	started, release, finished := make(chan struct{}, 1), make(chan struct{}), make(chan struct{})
	var releaseOnce, finishOnce sync.Once
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := relay.WSAccept(w, r)
		if err != nil {
			return
		}
		defer connection.Close()
		connection.SetReadDeadline(5 * time.Second)
		if _, err := connection.ReadFrame(); err != nil {
			return
		}
		select {
		case started <- struct{}{}:
		default:
		}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		_ = connection.WriteFrame(relay.WSOpText, []byte(`{"type":"response.done","response":{"usage":{"input_tokens":100,"output_tokens":20}}}`))
		select {
		case <-finished:
		case <-r.Context().Done():
		}
	}))
	defer upstream.Close()
	defer finishOnce.Do(func() { close(finished) })
	defer releaseOnce.Do(func() { close(release) })
	createChannelID(t, env, map[string]any{"name": "Frozen realtime", "key": upstreamKey, "base_url": upstream.URL, "models": "gpt-4o"})
	connection, err := relay.DialWebSocket("ws"+strings.TrimPrefix(env.base, "http")+"/v1/realtime?model=gpt-4o", http.Header{"Authorization": []string{"Bearer " + env.apiKey}})
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	connection.SetReadDeadline(5 * time.Second)
	if err := connection.WriteFrame(relay.WSOpText, []byte("synthetic-local-input")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("realtime upstream did not receive input")
	}
	setOption(t, env, "model_price", `{"gpt-4o":0.02}`)
	releaseOnce.Do(func() { close(release) })
	frame, err := connection.ReadFrame()
	if err != nil || !strings.Contains(string(frame.Data), "response.done") {
		t.Fatalf("realtime usage frame: %s %v", frame.Data, err)
	}
	_ = connection.Close()
	finishOnce.Do(func() { close(finished) })
	deadline := time.Now().Add(5 * time.Second)
	for {
		logs, count, err := model.ListLogs(env.rootID, 1, 10, "")
		if err != nil {
			t.Fatal(err)
		}
		if count == 1 {
			if logs[0].Quota != 5000 {
				t.Fatalf("realtime session was repriced: %+v", logs[0])
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("realtime session did not settle")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if current := relay.ComputeUsageQuota(relay.NewPriceData("gpt-4o", "default"), &relay.Usage{PromptTokens: 100, CompletionTokens: 20}); current != 10000 {
		t.Fatalf("current realtime price=%d,want10000", current)
	}
	user, err := model.GetUserByID(env.rootID)
	if err != nil || user.Quota != 100_000_000-5000 {
		t.Fatal("realtime reservation and settlement disagree")
	}
}

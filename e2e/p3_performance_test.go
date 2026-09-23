package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"starhub/model"
	"starhub/relay"
)

const p3ProfileRuns = 20

// TestP3PerformanceProfile measures representative P3 relay paths against
// local upstreams. It is opt-in so normal test runs remain fast and stable.
func TestP3PerformanceProfile(t *testing.T) {
	if os.Getenv("STARHUB_P3_PROFILE") != "1" {
		t.Skip("set STARHUB_P3_PROFILE=1 to run the local P3 latency profile")
	}

	for _, tc := range []struct {
		name  string
		model string
		kind  int
	}{
		{"chat_claude", "claude-3-5-sonnet", 14},
		{"chat_gemini", "gemini-1.5-flash", 25},
		{"chat_aws", "anthropic.claude-3-5-sonnet", 15},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var upstream *httptest.Server
			var key string
			switch tc.kind {
			case 14:
				upstream, _ = newClaudeUpstream(t)
				key = claudeUpstreamKey
			case 25:
				upstream = newGeminiUpstream(t)
				key = geminiUpstreamKey
			case 15:
				upstream, _ = newBedrockUpstream(t)
				key = bedrockAccessKey + "|" + bedrockSecretKey + "|" + bedrockRegion
			}
			env := setupRelayApp(t)
			createChannelID(t, env, map[string]any{
				"name": tc.name, "type": tc.kind, "key": key,
				"base_url": upstream.URL, "models": tc.model,
			})
			setP3ProfileUnlimited(t, env)
			payload := map[string]any{
				"model":    tc.model,
				"messages": []map[string]string{{"role": "user", "content": "hi"}},
			}
			profileP3Route(t, tc.name, func() apiResult {
				return doRequest(env.base, http.MethodPost, "/v1/chat/completions", env.apiKey, payload)
			})
		})
	}

	for _, tc := range []struct {
		name    string
		model   string
		path    string
		payload map[string]any
	}{
		{"embeddings", "text-embedding-3-small", "/v1/embeddings", map[string]any{"model": "text-embedding-3-small", "input": []string{"hello", "world"}}},
		{"rerank", "rerank-model", "/v1/rerank", map[string]any{"model": "rerank-model", "query": "hello", "documents": []string{"one", "two"}}},
		{"audio_tts", "tts-1", "/v1/audio/speech", map[string]any{"model": "tts-1", "input": "say hello", "voice": "alloy"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := newOpenAIExtraUpstream(t)
			env := setupRelayApp(t)
			createChannelID(t, env, map[string]any{
				"name": tc.name, "key": upstreamKey, "base_url": upstream.URL, "models": tc.model,
			})
			setP3ProfileUnlimited(t, env)
			profileP3Route(t, tc.name, func() apiResult {
				return doRequest(env.base, http.MethodPost, tc.path, env.apiKey, tc.payload)
			})
		})
	}

	t.Run("audio_stt", func(t *testing.T) {
		upstream := newOpenAIExtraUpstream(t)
		env := setupRelayApp(t)
		createChannelID(t, env, map[string]any{
			"name": "audio_stt", "key": upstreamKey, "base_url": upstream.URL, "models": "whisper-1",
		})
		setP3ProfileUnlimited(t, env)
		body, contentType := buildMultipartBody(t, map[string]string{"model": "whisper-1"},
			"file", "audio.mp3", []byte("fake-audio-bytes"))
		raw, err := io.ReadAll(body)
		if err != nil {
			t.Fatal(err)
		}
		profileP3Route(t, "audio_stt", func() apiResult {
			return postRaw(env.base, "/v1/audio/transcriptions", env.apiKey, contentType, bytes.NewReader(raw))
		})
	})

	t.Run("image_generation", func(t *testing.T) {
		profileP3Image(t, false)
	})
	t.Run("image_edit", func(t *testing.T) {
		profileP3Image(t, true)
	})

	t.Run("video_submit", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc((&videoTaskMock{
			sequence: []string{"completed"}, videoURL: "https://cdn.example/video.mp4",
		}).handler))
		t.Cleanup(upstream.Close)
		env := setupRelayApp(t)
		createChannelID(t, env, map[string]any{
			"name": "video_submit", "key": upstreamKey, "base_url": upstream.URL, "models": "sora-2",
		})
		setP3ProfileUnlimited(t, env)
		profileP3Route(t, "video_submit", func() apiResult {
			return doRequest(env.base, http.MethodPost, "/v1/video/generations", env.apiKey,
				map[string]any{"model": "sora-2", "prompt": "a cat", "seconds": 4, "n": 1})
		})
	})

	t.Run("video_poll", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc((&videoTaskMock{
			sequence: []string{"completed"}, videoURL: "https://cdn.example/video.mp4",
		}).handler))
		t.Cleanup(upstream.Close)
		env := setupRelayApp(t)
		createChannelID(t, env, map[string]any{
			"name": "video_poll", "key": upstreamKey, "base_url": upstream.URL, "models": "sora-2",
		})
		setP3ProfileUnlimited(t, env)
		paths := make([]string, p3ProfileRuns+2)
		for i := range paths {
			res := doRequest(env.base, http.MethodPost, "/v1/video/generations", env.apiKey,
				map[string]any{"model": "sora-2", "prompt": "a cat", "seconds": 4, "n": 1})
			if res.Status != http.StatusOK {
				t.Fatalf("video setup failed: %d %s", res.Status, res.Body)
			}
			var task struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal([]byte(res.Body), &task); err != nil || task.ID == "" {
				t.Fatalf("video setup returned no task ID: %s", res.Body)
			}
			paths[i] = "/v1/video/generations/" + task.ID
		}
		next := 0
		profileP3Route(t, "video_poll", func() apiResult {
			path := paths[next]
			next++
			return doRequest(env.base, http.MethodGet, path, env.apiKey, nil)
		})
	})

	t.Run("realtime", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := relay.WSAccept(w, r)
			if err != nil {
				return
			}
			defer conn.Close()
			if _, err := conn.ReadFrame(); err != nil {
				return
			}
			_ = conn.WriteFrame(relay.WSOpText, []byte(`{"type":"response.done","response":{"usage":{"input_tokens":1,"output_tokens":1}}}`))
		}))
		t.Cleanup(upstream.Close)
		env := setupRelayApp(t)
		createChannelID(t, env, map[string]any{
			"name": "realtime", "key": upstreamKey, "base_url": upstream.URL, "models": "gpt-4o-realtime",
		})
		setP3ProfileUnlimited(t, env)
		wsURL := "ws" + strings.TrimPrefix(env.base, "http") + "/v1/realtime?model=gpt-4o-realtime"
		profileP3Route(t, "realtime", func() apiResult {
			client, err := relay.DialWebSocket(wsURL, http.Header{
				"Authorization": []string{"Bearer " + env.apiKey},
			})
			if err != nil {
				return apiResult{Status: -1, Body: err.Error()}
			}
			defer client.Close()
			if err := client.WriteFrame(relay.WSOpText, []byte("hi")); err != nil {
				return apiResult{Status: -1, Body: err.Error()}
			}
			client.SetReadDeadline(5 * time.Second)
			frame, err := client.ReadFrame()
			if err != nil {
				return apiResult{Status: -1, Body: err.Error()}
			}
			return apiResult{Status: http.StatusOK, Body: string(frame.Data)}
		})
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			count, err := model.CountLogs(env.rootID)
			if err != nil {
				t.Fatal(err)
			}
			if count >= p3ProfileRuns+2 {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		count, _ := model.CountLogs(env.rootID)
		t.Fatalf("realtime sessions settled %d times, want %d", count, p3ProfileRuns+2)
	})
}

func setP3ProfileUnlimited(t *testing.T, env p1Env) {
	t.Helper()
	if err := model.UpdateTokenFields(env.tokenID, map[string]any{
		"unlimited_quota": true, "remain_quota": 0,
	}); err != nil {
		t.Fatal(err)
	}
}

func profileP3Image(t *testing.T, edit bool) {
	t.Helper()
	encoded := base64.StdEncoding.EncodeToString(make([]byte, 1_613_815))
	response := fmt.Sprintf(`{"created":0,"data":[{"b64_json":%q}]}`, encoded)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, response)
	}))
	t.Cleanup(upstream.Close)
	env := setupRelayApp(t)
	createChannelID(t, env, map[string]any{
		"name": "image_profile", "key": upstreamKey, "base_url": upstream.URL, "models": "gpt-image-2",
	})
	setP3ProfileUnlimited(t, env)
	setOption(t, env, "model_price", `{"gpt-image-2":0.04}`)
	if !edit {
		profileP3Route(t, "image_generation", func() apiResult {
			return doRequest(env.base, http.MethodPost, "/v1/images/generations", env.apiKey,
				map[string]any{"model": "gpt-image-2", "prompt": "a blue mug", "n": 1})
		})
		return
	}
	body, contentType := buildMultipartBody(t, map[string]string{
		"model": "gpt-image-2", "prompt": "make it blue", "n": "1",
	}, "image", "input.png", make([]byte, 1_613_815))
	raw, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	profileP3Route(t, "image_edit", func() apiResult {
		return postRaw(env.base, "/v1/images/edits", env.apiKey, contentType, bytes.NewReader(raw))
	})
}

func profileP3Route(t *testing.T, name string, request func() apiResult) {
	t.Helper()
	for i := 0; i < 2; i++ {
		res := request()
		if res.Status != http.StatusOK {
			t.Fatalf("%s warmup failed: %d %.200s", name, res.Status, res.Body)
		}
	}
	times := make([]float64, p3ProfileRuns)
	responseBytes := 0
	var beforeAlloc, afterAlloc runtime.MemStats
	runtime.ReadMemStats(&beforeAlloc)
	for i := range times {
		start := time.Now()
		res := request()
		times[i] = float64(time.Since(start)) / float64(time.Millisecond)
		if res.Status != http.StatusOK {
			t.Fatalf("%s request %d failed: %d %.200s", name, i+1, res.Status, res.Body)
		}
		responseBytes = len(res.Body)
	}
	runtime.ReadMemStats(&afterAlloc)
	sort.Float64s(times)
	var sum float64
	for _, ms := range times {
		sum += ms
	}
	median := (times[p3ProfileRuns/2-1] + times[p3ProfileRuns/2]) / 2
	p95 := times[int(math.Ceil(float64(p3ProfileRuns)*0.95))-1]
	allocKiBPerRequest := float64(afterAlloc.TotalAlloc-beforeAlloc.TotalAlloc) / 1024 / float64(p3ProfileRuns)
	t.Logf("P3_PROFILE name=%s runs=%d success=%d median_ms=%.3f p95_ms=%.3f mean_ms=%.3f min_ms=%.3f max_ms=%.3f response_bytes=%d process_alloc_kib_per_request=%.1f",
		name, p3ProfileRuns, p3ProfileRuns, median, p95, sum/float64(p3ProfileRuns), times[0], times[len(times)-1], responseBytes, allocKiBPerRequest)
}

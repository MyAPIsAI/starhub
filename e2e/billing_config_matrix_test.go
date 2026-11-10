package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"starhub/common"
	"starhub/model"
	"starhub/relay"
)

func assertBillingBalanceAndLog(t *testing.T, env p1Env, modelName string, spent, lastBill int64, logCount int64) {
	t.Helper()
	token, err := model.GetTokenByID(env.tokenID)
	if err != nil {
		t.Fatal(err)
	}
	if want := common.DefaultTokenQuota - spent; token.RemainQuota != want {
		t.Fatalf("token quota = %d, want %d", token.RemainQuota, want)
	}
	logs, total, err := model.ListLogs(env.rootID, 1, 10, modelName)
	if err != nil || total != logCount || len(logs) == 0 {
		t.Fatalf("logs for %s: total=%d err=%v", modelName, total, err)
	}
	if logs[0].Quota != lastBill {
		t.Fatalf("latest %s bill = %d, want %d", modelName, logs[0].Quota, lastBill)
	}
}

func TestBillingConfigExpressionAcrossRelayModes(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+upstreamKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"id":"priced-chat","choices":[{"index":0,"message":{"role":"assistant","content":"done"}}],"usage":{"prompt_tokens":100,"completion_tokens":50}}`)
		case "/v1/embeddings":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"data":[{"embedding":[0.1],"index":0}],"usage":{"prompt_tokens":123,"total_tokens":123}}`)
		case "/v1/rerank":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"results":[{"index":0,"relevance_score":0.9}],"usage":{"input_tokens":77,"output_tokens":3}}`)
		case "/v1/audio/speech":
			w.Header().Set("Content-Type", "audio/mpeg")
			fmt.Fprint(w, "priced-audio")
		case "/v1/audio/transcriptions", "/v1/audio/translations":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"text":"priced transcript","duration":6}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(upstream.Close)

	env := setupRelayApp(t)
	createChannelID(t, env, map[string]any{
		"name": "priced-modes", "key": upstreamKey, "base_url": upstream.URL,
		"models": "gpt-4o-mini,text-embedding-3-small,rerank-model,tts-1,whisper-1",
	})
	setOption(t, env, "model_price", `{"gpt-4o-mini":0.09,"text-embedding-3-small":0.09,"rerank-model":0.09,"tts-1":0.09,"whisper-1":0.09}`)
	setOption(t, env, "model_price_expr", `{"gpt-4o-mini":"prompt_tokens * 0.0001 + completion_tokens * 0.0002","text-embedding-3-small":"prompt_tokens * 0.00001","rerank-model":"prompt_tokens * 0.00001 + completion_tokens * 0.0001","tts-1":"0.02","whisper-1":"0.03"}`)

	spent := int64(0)
	res := doRequest(env.base, http.MethodPost, "/v1/chat/completions", env.apiKey, map[string]any{
		"model": "gpt-4o-mini", "messages": []map[string]string{{"role": "user", "content": "hi"}},
	})
	if res.Status != http.StatusOK {
		t.Fatalf("chat: %d %s", res.Status, res.Body)
	}
	spent += 10000
	assertBillingBalanceAndLog(t, env, "gpt-4o-mini", spent, 10000, 1)

	res = doRequest(env.base, http.MethodPost, "/v1/embeddings", env.apiKey, map[string]any{
		"model": "text-embedding-3-small", "input": []string{"hello", "world"},
	})
	if res.Status != http.StatusOK {
		t.Fatalf("embeddings: %d %s", res.Status, res.Body)
	}
	spent += 615
	assertBillingBalanceAndLog(t, env, "text-embedding-3-small", spent, 615, 1)

	res = doRequest(env.base, http.MethodPost, "/v1/rerank", env.apiKey, map[string]any{
		"model": "rerank-model", "query": "hello", "documents": []string{"one", "two"},
	})
	if res.Status != http.StatusOK {
		t.Fatalf("rerank: %d %s", res.Status, res.Body)
	}
	spent += 535
	assertBillingBalanceAndLog(t, env, "rerank-model", spent, 535, 1)

	res = doRequest(env.base, http.MethodPost, "/v1/audio/speech", env.apiKey, map[string]any{
		"model": "tts-1", "input": "hello", "voice": "alloy",
	})
	if res.Status != http.StatusOK || res.Body != "priced-audio" {
		t.Fatalf("speech: %d %s", res.Status, res.Body)
	}
	spent += 10000
	assertBillingBalanceAndLog(t, env, "tts-1", spent, 10000, 1)

	for _, path := range []string{"/v1/audio/transcriptions", "/v1/audio/translations"} {
		body, contentType := buildMultipartBody(t, map[string]string{
			"model": "whisper-1", "response_format": "verbose_json",
		}, "file", "audio.mp3", []byte("fake-audio"))
		res = postRaw(env.base, path, env.apiKey, contentType, body)
		if res.Status != http.StatusOK || !strings.Contains(res.Body, "priced transcript") {
			t.Fatalf("%s: %d %s", path, res.Status, res.Body)
		}
		spent += 15000
		logCount := int64(1)
		if path == "/v1/audio/translations" {
			logCount = 2
		}
		assertBillingBalanceAndLog(t, env, "whisper-1", spent, 15000, logCount)
	}
}

func TestBillingConfigSTTDurationRepricesEstimate(t *testing.T) {
	arrived := make(chan struct{})
	release := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/audio/transcriptions" {
			close(arrived)
			<-release
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"text":"six seconds","duration":6}`)
	}))
	t.Cleanup(upstream.Close)

	env := setupRelayApp(t)
	createChannelID(t, env, map[string]any{
		"name": "duration-audio", "key": upstreamKey, "base_url": upstream.URL,
		"models": "whisper-1",
	})
	setOption(t, env, "model_ratio", `{"whisper-1":2}`)
	body, contentType := buildMultipartBody(t, map[string]string{
		"model": "whisper-1", "response_format": "verbose_json",
	}, "file", "audio.mp3", make([]byte, 128000))
	result := make(chan apiResult, 1)
	go func() {
		result <- postRaw(env.base, "/v1/audio/transcriptions", env.apiKey, contentType, body)
	}()
	select {
	case <-arrived:
	case <-time.After(10 * time.Second):
		t.Fatal("transcription did not reach upstream")
	}
	token, err := model.GetTokenByID(env.tokenID)
	if err != nil || token.RemainQuota != common.DefaultTokenQuota-200 {
		t.Fatalf("duration precharge: token=%+v err=%v", token, err)
	}
	close(release)
	select {
	case res := <-result:
		if res.Status != http.StatusOK {
			t.Fatalf("transcription: %d %s", res.Status, res.Body)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("transcription did not finish")
	}
	assertBillingBalanceAndLog(t, env, "whisper-1", 150, 150, 1)

	body, contentType = buildMultipartBody(t, map[string]string{
		"model": "whisper-1", "response_format": "verbose_json",
	}, "file", "audio.mp3", make([]byte, 128000))
	res := postRaw(env.base, "/v1/audio/translations", env.apiKey, contentType, body)
	if res.Status != http.StatusOK {
		t.Fatalf("translation: %d %s", res.Status, res.Body)
	}
	assertBillingBalanceAndLog(t, env, "whisper-1", 300, 150, 2)
}

func TestBillingConfigChatCacheAndAudioDetails(t *testing.T) {
	usage := `{"prompt_tokens":100,"completion_tokens":40,"prompt_tokens_details":{"cached_tokens":30,"audio_tokens":20},"completion_tokens_details":{"audio_tokens":10}}`
	upstream := newP2Upstream(t, usage, usage, 0)
	env := setupRelayApp(t)
	createChannelID(t, env, map[string]any{
		"name": "detail-chat", "key": upstreamKey, "base_url": upstream.URL,
		"models": "gpt-4o-mini",
	})
	setOption(t, env, "model_ratio", `{"gpt-4o-mini":2}`)
	setOption(t, env, "completion_ratio", `{"gpt-4o-mini":3}`)
	setOption(t, env, "cache_ratio", `0.2`)
	setOption(t, env, "audio_ratio", `5`)
	setOption(t, env, "group_ratio", `{"default":1.5}`)
	res := doRequest(env.base, http.MethodPost, "/v1/chat/completions", env.apiKey, map[string]any{
		"model": "gpt-4o-mini", "messages": []map[string]string{{"role": "user", "content": "hi"}},
	})
	if res.Status != http.StatusOK {
		t.Fatalf("chat with usage details: %d %s", res.Status, res.Body)
	}
	assertBillingBalanceAndLog(t, env, "gpt-4o-mini", 444, 444, 1)
}

func TestBillingConfigExpressionAcrossChatProviders(t *testing.T) {
	type providerCase struct {
		name        string
		modelName   string
		channelType int
		key         string
		newUpstream func(*testing.T) *httptest.Server
	}
	cases := []providerCase{
		{
			name: "claude", modelName: "claude-3-5-sonnet", channelType: 14, key: claudeUpstreamKey,
			newUpstream: func(t *testing.T) *httptest.Server {
				server, _ := newClaudeUpstream(t)
				return server
			},
		},
		{
			name: "gemini", modelName: "gemini-1.5-flash", channelType: 25, key: geminiUpstreamKey,
			newUpstream: newGeminiUpstream,
		},
		{
			name: "bedrock", modelName: "anthropic.claude-3-5-sonnet", channelType: 15,
			key: bedrockAccessKey + "|" + bedrockSecretKey + "|" + bedrockRegion,
			newUpstream: func(t *testing.T) *httptest.Server {
				server, _ := newBedrockUpstream(t)
				return server
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			upstream := tc.newUpstream(t)
			env := setupRelayApp(t)
			createChannelID(t, env, map[string]any{
				"name": tc.name, "type": tc.channelType, "key": tc.key,
				"base_url": upstream.URL, "models": tc.modelName,
			})
			setOption(t, env, "model_price", fmt.Sprintf(`{%q:0.09}`, tc.modelName))
			setOption(t, env, "model_price_expr", fmt.Sprintf(`{%q:%q}`,
				tc.modelName, "prompt_tokens * 0.0001 + completion_tokens * 0.0002"))
			res := doRequest(env.base, http.MethodPost, "/v1/chat/completions", env.apiKey, map[string]any{
				"model":    tc.modelName,
				"messages": []map[string]string{{"role": "user", "content": "hi"}},
			})
			if res.Status != http.StatusOK {
				t.Fatalf("%s chat: %d %s", tc.name, res.Status, res.Body)
			}
			assertBillingBalanceAndLog(t, env, tc.modelName, 10000, 10000, 1)
		})
	}
}

func TestBillingConfigRealtimeExpression(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := relay.WSAccept(w, r)
		if err != nil {
			return
		}
		defer conn.Close()
		conn.SetReadDeadline(5 * time.Second)
		if _, err := conn.ReadFrame(); err != nil {
			return
		}
		_ = conn.WriteFrame(relay.WSOpText, []byte(`{"type":"response.done","response":{"usage":{"input_tokens":200,"output_tokens":100}}}`))
		time.Sleep(100 * time.Millisecond)
	}))
	t.Cleanup(upstream.Close)

	env := setupRelayApp(t)
	createChannelID(t, env, map[string]any{
		"name": "priced-realtime", "key": upstreamKey, "base_url": upstream.URL,
		"models": "gpt-4o-realtime",
	})
	setOption(t, env, "model_price", `{"gpt-4o-realtime":0.09}`)
	setOption(t, env, "model_price_expr", `{"gpt-4o-realtime":"0.02"}`)
	wsURL := "ws" + strings.TrimPrefix(env.base, "http") + "/v1/realtime?model=gpt-4o-realtime"
	client, err := relay.DialWebSocket(wsURL, http.Header{
		"Authorization": []string{"Bearer " + env.apiKey},
	})
	if err != nil {
		t.Fatalf("realtime dial: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	token, err := model.GetTokenByID(env.tokenID)
	if err != nil || token.RemainQuota != common.DefaultTokenQuota-10000 {
		t.Fatalf("realtime precharge: token=%+v err=%v", token, err)
	}
	if err := client.WriteFrame(relay.WSOpText, []byte("hi")); err != nil {
		t.Fatalf("realtime write: %v", err)
	}
	client.SetReadDeadline(5 * time.Second)
	frame, err := client.ReadFrame()
	if err != nil || !strings.Contains(string(frame.Data), `"input_tokens":200`) {
		t.Fatalf("realtime usage frame: %s err=%v", frame.Data, err)
	}
	_ = client.Close()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		logs, total, err := model.ListLogs(env.rootID, 1, 10, "gpt-4o-realtime")
		if err == nil && total == 1 && logs[0].Quota == 10000 {
			assertBillingBalanceAndLog(t, env, "gpt-4o-realtime", 10000, 10000, 1)
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("realtime expression bill was not settled")
}

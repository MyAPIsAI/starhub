package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBillingConfigTierBoundaries(t *testing.T) {
	cases := []struct {
		name   string
		prompt int64
		quota  int64
	}{
		{name: "below-32k", prompt: 31990, quota: 9777},
		{name: "at-32k", prompt: 32000, quota: 9780},
		{name: "above-32k", prompt: 32001, quota: 14670},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":%d,"completion_tokens":100}}`, tc.prompt)
			}))
			t.Cleanup(upstream.Close)
			env := setupRelayApp(t)
			createChannelID(t, env, map[string]any{
				"name": "tier-boundary", "key": upstreamKey, "base_url": upstream.URL,
				"models": "gpt-4o-mini",
			})
			setOption(t, env, "model_price_expr", `{"gpt-4o-mini":"prompt_tokens <= 32000 ? (prompt_tokens * 0.6 + completion_tokens * 3.6) / 1000000 : (prompt_tokens * 0.9 + completion_tokens * 5.4) / 1000000"}`)
			res := doRequest(env.base, http.MethodPost, "/v1/chat/completions", env.apiKey, map[string]any{
				"model": "gpt-4o-mini", "messages": []map[string]string{{"role": "user", "content": "hi"}},
			})
			if res.Status != http.StatusOK {
				t.Fatalf("tier request: %d %s", res.Status, res.Body)
			}
			assertBillingBalanceAndLog(t, env, "gpt-4o-mini", tc.quota, tc.quota, 1)
		})
	}
}

func TestBillingConfigTTSVariableExpressionUsesZeroTokens(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		fmt.Fprint(w, "audio")
	}))
	t.Cleanup(upstream.Close)
	env := setupRelayApp(t)
	createChannelID(t, env, map[string]any{
		"name": "tts-variable", "key": upstreamKey, "base_url": upstream.URL,
		"models": "tts-1",
	})
	setOption(t, env, "model_price", `{"tts-1":0.09}`)
	setOption(t, env, "model_price_expr", `{"tts-1":"prompt_tokens * 0.01"}`)
	res := doRequest(env.base, http.MethodPost, "/v1/audio/speech", env.apiKey, map[string]any{
		"model": "tts-1", "input": "a text input that has tokens", "voice": "alloy",
	})
	if res.Status != http.StatusOK {
		t.Fatalf("tts request: %d %s", res.Status, res.Body)
	}
	assertBillingBalanceAndLog(t, env, "tts-1", 0, 0, 1)
}

func TestBillingConfigSTTVariableExpressionWithoutDuration(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"text":"transcript without duration"}`)
	}))
	t.Cleanup(upstream.Close)
	env := setupRelayApp(t)
	createChannelID(t, env, map[string]any{
		"name": "stt-no-duration", "key": upstreamKey, "base_url": upstream.URL,
		"models": "whisper-1",
	})
	setOption(t, env, "model_price_expr", `{"whisper-1":"prompt_tokens * 0.01"}`)
	body, contentType := buildMultipartBody(t, map[string]string{
		"model": "whisper-1", "response_format": "verbose_json",
	}, "file", "audio.mp3", make([]byte, 128000))
	res := postRaw(env.base, "/v1/audio/transcriptions", env.apiKey, contentType, body)
	if res.Status != http.StatusOK {
		t.Fatalf("stt request: %d %s", res.Status, res.Body)
	}
	assertBillingBalanceAndLog(t, env, "whisper-1", 0, 0, 1)
}

func TestBillingConfigUnknownVariableFallsBackToFixedAtSettlement(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":100,"completion_tokens":50}}`)
	}))
	t.Cleanup(upstream.Close)
	env := setupRelayApp(t)
	createChannelID(t, env, map[string]any{
		"name": "bad-variable", "key": upstreamKey, "base_url": upstream.URL,
		"models": "gpt-4o-mini",
	})
	setOption(t, env, "model_price", `{"gpt-4o-mini":0.02}`)
	setOption(t, env, "model_price_expr", `{"gpt-4o-mini":"unknown_meter * 0.01"}`)
	res := doRequest(env.base, http.MethodPost, "/v1/chat/completions", env.apiKey, map[string]any{
		"model": "gpt-4o-mini", "messages": []map[string]string{{"role": "user", "content": "hi"}},
	})
	if res.Status != http.StatusOK {
		t.Fatalf("bad-variable request: %d %s", res.Status, res.Body)
	}
	assertBillingBalanceAndLog(t, env, "gpt-4o-mini", 10000, 10000, 1)
}

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"starhub/model"
	"starhub/setting"
)

func TestResourceGatewayTextProfileRejectsUndeclaredModesWithoutUpstreamOrFees(t *testing.T) {
	env := setupRelayApp(t)
	var calls atomic.Int64
	models := make(chan string, 10)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var input struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&input)
		models <- input.Model
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"local-profile","object":"chat.completion","model":%q,"choices":[{"index":0,"message":{"role":"assistant","content":"local"}}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`, input.Model)
	}))
	t.Cleanup(upstream.Close)
	ch := &model.Channel{Name: "managed text", Type: 1, Key: "synthetic", BaseURL: upstream.URL, Models: "alpha", Group: "default", Status: 1, Priority: 10}
	if err := model.InsertChannel(ch); err != nil {
		t.Fatal(err)
	}
	if err := model.DB.Create(&model.ResourceGatewayManagedChannel{ChannelID: ch.ID, SourceChannelID: "fixture", Active: true, Profile: model.ResourceGatewayTextProfile}).Error; err != nil {
		t.Fatal(err)
	}
	if err := model.DB.Create(&model.ResourceGatewayRoute{ChannelID: ch.ID, Model: "alpha", Group: "default", Capabilities: `["text"]`, ContextLength: 200}).Error; err != nil {
		t.Fatal(err)
	}
	plain := func(model string) map[string]any {
		return map[string]any{"model": model, "messages": []map[string]string{{"role": "user", "content": "plain"}}, "max_tokens": 10}
	}
	for _, request := range []struct {
		path string
		body any
	}{{"/v1/chat/completions", plain("alpha")}, {"/v1/chat/completions", plain("myapis-auto")}, {"/v1/messages", plain("alpha")}} {
		res := doRequest(env.base, "POST", request.path, env.apiKey, request.body)
		if res.Status != 200 {
			t.Fatalf("basic text failed: %s %d %s", request.path, res.Status, res.Body)
		}
		if name := <-models; name != "alpha" {
			t.Fatalf("virtual model leaked: %s", name)
		}
	}
	requests := []struct{ method, path, body string }{
		{"POST", "/v1/chat/completions", `{"model":"alpha","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,c3ludGhldGlj"}}]}],"max_tokens":10}`},
		{"POST", "/v1/chat/completions", `{"model":"alpha","messages":[{"role":"user","content":[{"type":"input_audio","input_audio":{"data":"synthetic","format":"wav"}}]}],"max_tokens":10}`},
		{"POST", "/v1/chat/completions", `{"model":"alpha","messages":[{"role":"user","content":[{"type":"text","text":"plain","image_url":"hidden"}]}],"max_tokens":10}`},
		{"POST", "/v1/chat/completions", `{"model":"alpha","messages":[{"role":"user","content":"plain","audio":{"data":"hidden"}}],"max_tokens":10}`},
		{"POST", "/v1/chat/completions", `{"model":"alpha","messages":[{"role":"user","content":"plain"}],"modalities":["audio"],"max_tokens":10}`},
		{"POST", "/v1/chat/completions", `{"model":"alpha","messages":[{"role":"user","content":"plain"}],"extra_body":{"audio":"hidden"},"max_tokens":10}`},
		{"POST", "/v1/chat/completions", `{"model":"alpha","messages":[],"messages":[{"role":"user","content":"plain"}],"max_tokens":10}`},
		{"POST", "/v1/chat/completions", `{"model":"alpha","messages":[{"role":"user","content":"plain"}],"tools":[{"type":"function","function":{"name":"echo","parameters":{"type":"object"}}}],"max_tokens":10}`},
		{"POST", "/v1/chat/completions", `{"model":"alpha","messages":[{"role":"user","content":"plain"}],"max_tokens":201}`},
		{"POST", "/v1/chat/completions", `{"model":"alpha","messages":[{"role":"user","content":"` + strings.Repeat("word ", 1000) + `"}],"max_tokens":10}`},
		{"POST", "/v1/responses", `{"model":"alpha","input":"plain","store":false,"max_output_tokens":10}`},
		{"POST", "/v1/images/generations", `{"model":"alpha","prompt":"plain","n":1,"size":"1024x1024"}`},
		{"POST", "/v1/embeddings", `{"model":"alpha","input":"plain"}`},
		{"POST", "/v1/rerank", `{"model":"alpha","query":"plain","documents":["text"]}`},
		{"POST", "/v1/audio/speech", `{"model":"alpha","input":"plain","voice":"alloy"}`},
		{"POST", "/v1/video/generations", `{"model":"alpha","prompt":"plain","seconds":"4","size":"1280x720"}`},
		{"GET", "/v1/realtime?model=alpha", ""},
	}
	for i, request := range requests {
		before, err := model.GetUserByID(env.rootID)
		if err != nil {
			t.Fatal(err)
		}
		count := calls.Load()
		req, err := http.NewRequest(request.method, env.base+request.path, strings.NewReader(request.body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+env.apiKey)
		req.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if response.StatusCode < 400 {
			t.Fatalf("case%d undeclared request succeeded: %s %d %s", i, request.path, response.StatusCode, body)
		}
		after, err := model.GetUserByID(env.rootID)
		if err != nil || before.Quota != after.Quota || calls.Load() != count {
			t.Fatalf("case%d undeclared request charged or forwarded: %v %s", i, err, body)
		}
	}
	// The smart chooser must skip a cheaper text-only resource for tool calls,
	// rather than pinning it and failing after a suitable legacy model was missed.
	legacy := &model.Channel{Name: "legacy tools", Type: 1, Key: "synthetic", BaseURL: upstream.URL, Models: "beta", Group: "default", Status: 1}
	if err := model.InsertChannel(legacy); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"alpha", "beta"} {
		if err := model.UpsertModelMeta(&model.ModelMeta{Model: name, Tags: "function_call", ContextLength: 128000}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := model.SaveOption(setting.OptionKeyModelRatio, `{"alpha":0.01,"beta":1}`, env.rootID); err != nil {
		t.Fatal(err)
	}
	if err := model.LoadOptionsToSettings(); err != nil {
		t.Fatal(err)
	}
	res := doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, map[string]any{"model": "myapis-auto", "messages": []map[string]string{{"role": "user", "content": "tool request"}}, "tools": []map[string]any{{"type": "function", "function": map[string]any{"name": "echo", "parameters": map[string]any{"type": "object"}}}}, "max_tokens": 10})
	if res.Status != 200 {
		t.Fatalf("legacy tool fallback failed: %d %s", res.Status, res.Body)
	}
	if got := <-models; got != "beta" {
		t.Fatalf("smart tool request routed to text resource: %s", got)
	}
}

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"starhub/model"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

func TestResourceAdvancedActualRouterAliasAndCapabilities(t *testing.T) {
	env := setupRelayApp(t)
	var posts atomic.Int64
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&body)
		var name string
		_ = json.Unmarshal(body["model"], &name)
		if name != "provider-model" {
			t.Errorf("alias was not forwarded: %q", name)
		}
		posts.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"chat-fixture","model":"provider-model","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`)
	}))
	defer up.Close()
	id := createChannelID(t, env, map[string]any{"name": "advanced", "key": "synthetic", "base_url": up.URL, "models": "gpt-4o"})
	source := strconv.FormatInt(id, 10)
	snapshot := model.ResourceSnapshot{ID: "advanced", Sequence: 1, PublishedAt: "2026-10-29T00:00:00Z", Source: "local test", Channels: []model.ResourceChannel{{ID: source, Name: "Source", Provider: "Fixture", Enabled: true}}, Instances: []model.ResourceInstance{{ID: "advanced-model", ChannelID: source, UpstreamModel: "provider-model", CanonicalModel: "gpt-4o", Protocols: []string{"OpenAI"}, Capabilities: []string{"text", "vision", "function_call"}, Status: "available"}}}
	catalog, err := model.SaveResourceSnapshot(context.Background(), 0, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	name := "gpt-4o"
	_, err = model.SaveResourcePlacement(context.Background(), catalog.Revision, "advanced-model", model.ResourcePlacement{ModelID: &name, GroupIDs: []string{"default"}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := model.ReadResourcePublicationContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	bindings := []model.ResourceChannelBinding{{SourceChannelID: source, GatewayChannelID: source}}
	expected := model.ResourcePublicationPrecondition{ResourceRevision: ctx.ResourceRevision, ChannelRevision: ctx.ChannelRevision, OptionVersion: ctx.OptionVersion}
	plan, err := model.PreviewResourcePublication(context.Background(), expected, bindings)
	if err != nil || !plan.Ready {
		t.Fatalf("plan %+v %v", plan, err)
	}
	if _, err = model.PublishResourceGateway(context.Background(), expected, bindings, plan.ReviewDigest, 1); err != nil {
		t.Fatal(err)
	}
	call := func(path, body string) int {
		t.Helper()
		req, _ := http.NewRequest("POST", env.base+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+env.apiKey)
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode >= 500 {
			t.Logf("denied: %d %s", res.StatusCode, raw)
		}
		return res.StatusCode
	}
	for _, body := range []string{
		`{"model":"gpt-4o","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,AA=="}}]}],"max_tokens":10}`,
		`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}],"tools":[{"type":"function","function":{"name":"test","parameters":{"type":"object"}}}],"max_tokens":10}`,
	} {
		if got := call("/v1/chat/completions", body); got != 200 {
			t.Fatalf("declared capability rejected: %d", got)
		}
	}
	for _, body := range []string{
		`{"model":"gpt-4o","messages":[{"role":"user","content":[{"type":"input_audio","input_audio":{"data":"AA==","format":"wav"}}]}]}`,
		`{"model":"gpt-4o","messages":[{"role":"user","content":[{"type":"text","text":"hello","hidden_image":"bad"}]}]}`,
	} {
		if got := call("/v1/chat/completions", body); got == 200 {
			t.Fatal("undeclared input accepted")
		}
	}
	if got := call("/v1/responses", `{"model":"gpt-4o","input":"hello"}`); got == 200 {
		t.Fatal("undeclared endpoint accepted")
	}
	if posts.Load() != 2 {
		t.Fatalf("denied requests reached provider %d", posts.Load())
	}
}

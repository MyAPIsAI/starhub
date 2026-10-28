package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"starhub/model"
)

func TestResourceGatewayExactPairsProtectActualDiscoveryAndCalls(t *testing.T) {
	env := setupRelayApp(t)
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var request struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request.Model != "alpha" {
			t.Errorf("unpublished upstream model: %s", request.Model)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"local-managed","object":"chat.completion","model":"alpha","choices":[{"index":0,"message":{"role":"assistant","content":"local"}}],"usage":{"prompt_tokens":100,"completion_tokens":50}}`)
	}))
	t.Cleanup(upstream.Close)
	ch := &model.Channel{Name: "managed", Type: 1, Key: "synthetic-managed-key", BaseURL: upstream.URL, Models: "alpha,beta", Group: "default,vip", Status: 1}
	if err := model.InsertChannel(ch); err != nil {
		t.Fatal(err)
	}
	if err := model.DB.Create(&model.ResourceGatewayManagedChannel{ChannelID: ch.ID, SourceChannelID: "fixture-source", Active: true, Profile: model.ResourceGatewayTextProfile}).Error; err != nil {
		t.Fatal(err)
	}
	if err := model.DB.Create(&[]model.ResourceGatewayRoute{{ChannelID: ch.ID, Model: "alpha", Group: "default", Capabilities: `["text"]`, ContextLength: 128000}, {ChannelID: ch.ID, Model: "beta", Group: "vip", Capabilities: `["text"]`, ContextLength: 128000}}).Error; err != nil {
		t.Fatal(err)
	}
	models := func() string {
		t.Helper()
		res := doRequest(env.base, "GET", "/v1/models", env.apiKey, nil)
		if res.Status != 200 {
			t.Fatalf("models: %d %s", res.Status, res.Body)
		}
		return res.Body
	}
	if body := models(); !strings.Contains(body, `"alpha"`) || strings.Contains(body, `"beta"`) {
		t.Fatalf("group discovery leaked cross-product: %s", body)
	}
	deny := func(name string) {
		t.Helper()
		before, err := model.GetUserByID(env.rootID)
		if err != nil {
			t.Fatal(err)
		}
		beforeCalls := calls.Load()
		res := doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, map[string]any{"model": name, "messages": []map[string]string{{"role": "user", "content": "local fixture"}}, "max_tokens": 10})
		if res.Status < 400 {
			t.Fatalf("unauthorized %s reached gateway: %d %s", name, res.Status, res.Body)
		}
		after, err := model.GetUserByID(env.rootID)
		if err != nil || after.Quota != before.Quota || calls.Load() != beforeCalls {
			t.Fatalf("rejected %s touched quota/upstream: %v quota=%d/%d calls=%d/%d", name, err, after.Quota, before.Quota, calls.Load(), beforeCalls)
		}
	}
	deny("beta")
	for _, name := range []string{"alpha", "myapis-auto"} {
		res := doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, map[string]any{"model": name, "messages": []map[string]string{{"role": "user", "content": "local fixture"}}, "max_tokens": 10})
		if res.Status != 200 {
			t.Fatalf("published %s failed: %d %s", name, res.Status, res.Body)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("wrong local upstream count: %d", calls.Load())
	}
	// The ordinary management API cannot expand managed routes with a model/group cross-product.
	res := doRequest(env.base, "PUT", "/api/channel", env.session, map[string]any{"id": ch.ID, "models": "alpha,beta,injected", "group": "default,vip,other"})
	if res.Status != 200 {
		t.Fatalf("manual edit: %d %s", res.Status, res.Body)
	}
	deny("beta")
	deny("injected")
	if body := models(); strings.Contains(body, `"beta"`) || strings.Contains(body, `"injected"`) {
		t.Fatalf("manual edit broadened discovery: %s", body)
	}
	// Retained inactive ownership denies stale abilities even before a cleanup rebuild.
	if err := model.DB.Model(&model.ResourceGatewayManagedChannel{}).Where("channel_id = ?", ch.ID).Update("active", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := model.SetAbilitiesEnabledByChannel(ch.ID, true); err != nil {
		t.Fatal(err)
	}
	deny("alpha")
	if body := models(); strings.Contains(body, `"alpha"`) {
		t.Fatalf("revoked route remained discoverable: %s", body)
	}
	if res := doRequest(env.base, "GET", "/api/plaza/models/alpha", env.session, nil); res.Status == 200 {
		var body struct {
			Data struct {
				Model struct {
					Available bool `json:"available"`
				} `json:"model"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(res.Body), &body); err != nil || body.Data.Model.Available {
			t.Fatalf("revoked plaza route available: %v %s", err, res.Body)
		}
	}
}

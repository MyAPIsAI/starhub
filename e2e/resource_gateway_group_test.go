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

func TestResourceGatewayActualGroupRecreationCannotRestorePublishedGrant(t *testing.T) {
	env := setupRelayApp(t)
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"local-group","model":"alpha","choices":[{"index":0,"message":{"role":"assistant","content":"local"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	t.Cleanup(upstream.Close)
	ch := &model.Channel{Name: "managed", Type: 1, Key: "synthetic", BaseURL: upstream.URL, Models: "alpha", Group: "default", Status: 1}
	if err := model.InsertChannel(ch); err != nil {
		t.Fatal(err)
	}
	if err := model.DB.Create(&model.ResourceGatewayManagedChannel{ChannelID: ch.ID, SourceChannelID: "fixture", Active: true, Profile: model.ResourceGatewayTextProfile}).Error; err != nil {
		t.Fatal(err)
	}
	if err := model.DB.Create(&model.ResourceGatewayRoute{ChannelID: ch.ID, Model: "alpha", Group: "default", Capabilities: `["text"]`, ContextLength: 128000}).Error; err != nil {
		t.Fatal(err)
	}
	put := func() int64 {
		t.Helper()
		res := doRequest(env.base, "PUT", "/api/service-group", env.session, map[string]any{"group_id": "default", "name": "Default", "multiplier": "1", "enabled": true, "visibility": "common"})
		if res.Status != 200 {
			t.Fatalf("group save: %d %s", res.Status, res.Body)
		}
		var body struct {
			Data struct {
				ID int64 `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(res.Body), &body); err != nil {
			t.Fatal(err)
		}
		return body.Data.ID
	}
	first := put()
	if err := model.DB.Model(&model.ResourceGatewayRoute{}).Where("channel_id = ?", ch.ID).Update("group_definition_id", first).Error; err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{"model": "alpha", "messages": []map[string]string{{"role": "user", "content": "plain"}}, "max_tokens": 10}
	if res := doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, payload); res.Status != 200 {
		t.Fatalf("bound identity unavailable: %d %s", res.Status, res.Body)
	}
	before, err := model.GetUserByID(env.rootID)
	if err != nil {
		t.Fatal(err)
	}
	if res := doRequest(env.base, "DELETE", "/api/service-group/default", env.session, nil); res.Status != 200 {
		t.Fatalf("group delete: %d %s", res.Status, res.Body)
	}
	recreated := put()
	if recreated == first {
		t.Fatal("group identity was not recreated")
	}
	res := doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, payload)
	if res.Status < 400 {
		t.Fatalf("old grant attached to recreated group: %d %s", res.Status, res.Body)
	}
	res = doRequest(env.base, "GET", "/v1/models", env.apiKey, nil)
	if res.Status != 200 || strings.Contains(res.Body, `"alpha"`) {
		t.Fatalf("recreated group advertised old grant: %d %s", res.Status, res.Body)
	}
	after, err := model.GetUserByID(env.rootID)
	if err != nil || after.Quota != before.Quota || calls.Load() != 1 {
		t.Fatalf("recreated identity charged or forwarded: %v calls=%d", err, calls.Load())
	}
}

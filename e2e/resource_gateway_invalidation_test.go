package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"starhub/model"
)

func TestResourceGatewayActualInventoryEditRevokesInSameCommit(t *testing.T) {
	env := setupRelayApp(t)
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
	t.Cleanup(upstream.Close)
	ch := &model.Channel{Name: "managed", Type: 1, Key: "synthetic", BaseURL: upstream.URL, Models: "alpha", Group: "default", Status: 1}
	if err := model.InsertChannel(ch); err != nil {
		t.Fatal(err)
	}
	snapshot := model.ResourceSnapshot{ID: "actual-source", Sequence: 1, PublishedAt: "2026-10-29T00:00:00Z", Channels: []model.ResourceChannel{{ID: "supplier", Name: "Supplier", Enabled: true}}, Instances: []model.ResourceInstance{{ID: "resource", ChannelID: "supplier", CanonicalModel: "alpha", UpstreamModel: "alpha", Protocols: []string{"OpenAI"}, Capabilities: []string{"text"}, Status: "available"}}}
	state, err := model.SaveResourceSnapshot(context.Background(), 0, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	name := "alpha"
	placement := model.ResourcePlacement{ModelID: &name, GroupIDs: []string{"default"}, Enabled: true}
	state, err = model.SaveResourcePlacement(context.Background(), state.Revision, "resource", placement)
	if err != nil {
		t.Fatal(err)
	}
	if err := model.DB.Create(&model.ResourceGatewayManagedChannel{ChannelID: ch.ID, SourceChannelID: "supplier", Active: true, Profile: model.ResourceGatewayTextProfile}).Error; err != nil {
		t.Fatal(err)
	}
	if err := model.DB.Create(&model.ResourceGatewayRoute{ChannelID: ch.ID, Model: "alpha", Group: "default", Capabilities: `["text"]`, ContextLength: 128000}).Error; err != nil {
		t.Fatal(err)
	}
	before, err := model.GetUserByID(env.rootID)
	if err != nil {
		t.Fatal(err)
	}
	res := doRequest(env.base, "GET", "/api/resource", env.session, nil)
	var body struct {
		Data model.ResourceCatalog `json:"data"`
	}
	if err := json.Unmarshal([]byte(res.Body), &body); err != nil || !body.Data.AffectsGatewayRouting {
		t.Fatalf("managed effect hidden: %v %s", err, res.Body)
	}
	placement.Enabled = false
	res = doRequest(env.base, "PUT", "/api/placement/resource", env.session, map[string]any{"expected_revision": state.Revision, "placement": placement})
	if res.Status != 200 {
		t.Fatalf("disable: %d %s", res.Status, res.Body)
	}
	if err := json.Unmarshal([]byte(res.Body), &body); err != nil || !body.Data.AffectsGatewayRouting {
		t.Fatalf("revocation effect hidden: %v %s", err, res.Body)
	}
	var marker model.ResourceGatewayManagedChannel
	if err := model.DB.First(&marker, "channel_id = ?", ch.ID).Error; err != nil || marker.Active {
		t.Fatalf("placement commit did not revoke channel %s: %+v %v", strconv.FormatInt(ch.ID, 10), marker, err)
	}
	placement.Enabled = true
	res = doRequest(env.base, "PUT", "/api/placement/resource", env.session, map[string]any{"expected_revision": body.Data.Revision, "placement": placement})
	if res.Status != 200 {
		t.Fatalf("restore draft: %d %s", res.Status, res.Body)
	}
	res = doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, map[string]any{"model": "alpha", "messages": []map[string]string{{"role": "user", "content": "local fixture"}}, "max_tokens": 10})
	if res.Status < 400 {
		t.Fatalf("restored draft reopened publication: %d %s", res.Status, res.Body)
	}
	after, err := model.GetUserByID(env.rootID)
	if err != nil || after.Quota != before.Quota || calls.Load() != 0 {
		t.Fatalf("revoked call charged or sent: %v calls=%d", err, calls.Load())
	}
}

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"starhub/model"
)

func TestRESAC21SourceIdentityRequiresConfirmationBeforeCallsResume(t *testing.T) {
	env := setupRelayApp(t)
	var hits atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		var body struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"review","model":%q,"choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`, body.Model)
	}))
	defer upstream.Close()
	publicationAPIData[any](t, doRequest(env.base, "PUT", "/api/service-group", env.session, map[string]any{"group_id": "default", "name": "Default", "enabled": true, "visibility": "common", "multiplier": "1"}))
	channelID := createChannelID(t, env, map[string]any{"name": "Boundary fixture", "key": "synthetic", "type": 1, "base_url": upstream.URL, "models": "alpha", "group": "default", "status": 1})
	if err := model.DB.Create(&model.ResourceGatewayManagedChannel{ChannelID: channelID, SourceChannelID: "source", Active: false, Profile: model.ResourceGatewayTextProfile}).Error; err != nil {
		t.Fatal(err)
	}
	snapshot := model.ResourceSnapshot{ID: "boundary", Sequence: 1, PublishedAt: "2026-11-07T00:00:00Z", Channels: []model.ResourceChannel{{ID: "source", Name: "Source", Enabled: true}}, Instances: []model.ResourceInstance{{ID: "instance", ChannelID: "source", CanonicalModel: "alpha", UpstreamModel: "alpha", Protocols: []string{"OpenAI"}, Capabilities: []string{"text"}, ContextLength: 128000, Status: "available"}}}
	read := func() model.ResourceCatalog {
		return publicationAPIData[model.ResourceCatalog](t, doRequest(env.base, "GET", "/api/resource", env.session, nil))
	}
	importSnapshot := func() model.ResourceCatalog {
		return publicationAPIData[model.ResourceCatalog](t, doRequest(env.base, "PUT", "/api/resource", env.session, map[string]any{"expected_revision": read().Revision, "snapshot": snapshot}))
	}
	chat := func() apiResult {
		return doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, map[string]any{"model": "alpha", "messages": []map[string]string{{"role": "user", "content": "fixture"}}, "max_tokens": 10})
	}
	deny := func() {
		t.Helper()
		before, _ := model.GetUserByID(env.rootID)
		calls := hits.Load()
		response := chat()
		after, _ := model.GetUserByID(env.rootID)
		if response.Status < 400 || hits.Load() != calls || before.Quota != after.Quota {
			t.Fatalf("denied resource called or charged: %d %s", response.Status, response.Body)
		}
	}
	allow := func() {
		t.Helper()
		if response := chat(); response.Status != 200 {
			t.Fatalf("confirmed resource unavailable: %d %s", response.Status, response.Body)
		}
	}
	publish := func(source string) {
		t.Helper()
		state := publicationAPIData[model.ResourceBindingState](t, doRequest(env.base, "GET", "/api/resource/publication-state", env.session, nil))
		for _, binding := range state.Bindings {
			if binding.GatewayChannelID == strconv.FormatInt(channelID, 10) && binding.SourceChannelID != source {
				publicationAPIData[model.ResourceBindingChangeReceipt](t, doRequest(env.base, "PUT", "/api/resource/bindings/"+binding.GatewayChannelID, env.session, map[string]any{
					"expected_resource_revision": state.ResourceRevision, "expected_channel_revision": state.ChannelRevision,
					"expected_source_channel_id": binding.SourceChannelID, "expected_publication_id": binding.PublicationID,
					"operation": "release", "request_id": "0123456789abcdef0123456789abcdef",
				}))
				deny()
			}
		}
		context := publicationAPIData[model.ResourcePublicationContext](t, doRequest(env.base, "GET", "/api/resource/publication-context", env.session, nil))
		payload := map[string]any{"expected_resource_revision": context.ResourceRevision, "expected_channel_revision": context.ChannelRevision, "expected_option_version": context.OptionVersion, "bindings": []model.ResourceChannelBinding{{SourceChannelID: source, GatewayChannelID: strconv.FormatInt(channelID, 10)}}}
		preview := publicationAPIData[model.ResourcePublicationPreview](t, doRequest(env.base, "POST", "/api/resource/publication-preview", env.session, payload))
		if !preview.Ready {
			t.Fatalf("not ready: %+v", preview.Issues)
		}
		payload["review_digest"] = preview.ReviewDigest
		publicationAPIData[model.ResourcePublicationAcknowledgement](t, doRequest(env.base, "PUT", "/api/resource/publication", env.session, payload))
	}
	name := "alpha"
	placement := model.ResourcePlacement{ModelID: &name, GroupIDs: []string{"default"}, Enabled: true}
	state := importSnapshot()
	if state.Placements["instance"].ModelID != nil || state.Placements["instance"].Enabled {
		t.Fatal("new resource inherited authorization")
	}
	deny()
	publicationAPIData[model.ResourceCatalog](t, doRequest(env.base, "PUT", "/api/placement/instance", env.session, map[string]any{"expected_revision": state.Revision, "placement": placement}))
	publish("source")
	allow()
	for _, change := range []string{"upstream", "channel", "provider"} {
		t.Log("reviewing identity change", change)
		snapshot.Sequence++
		switch change {
		case "upstream":
			snapshot.Instances[0].UpstreamModel = "provider-alpha"
		case "channel":
			snapshot.Channels = append(snapshot.Channels, model.ResourceChannel{ID: "other", Name: "Other", Enabled: true})
			snapshot.Instances[0].ChannelID = "other"
		case "provider":
			snapshot.Channels[1].Provider = "New vendor"
		}
		state = importSnapshot()
		deny()
		if len(state.PlacementReviews) != 1 || state.Placements["instance"].Enabled {
			t.Fatal("identity change not held")
		}
		placement.ConfirmIdentity = false
		if response := doRequest(env.base, "PUT", "/api/placement/instance", env.session, map[string]any{"expected_revision": state.Revision, "placement": placement}); response.Status != 400 {
			t.Fatalf("unconfirmed restore: %d", response.Status)
		}
		placement.ConfirmIdentity = true
		publicationAPIData[model.ResourceCatalog](t, doRequest(env.base, "PUT", "/api/placement/instance", env.session, map[string]any{"expected_revision": state.Revision, "placement": placement}))
		deny()
		publish(snapshot.Instances[0].ChannelID)
		allow()
	}
	// External ID changes are removal plus new intake, never an authorization transfer.
	snapshot.Sequence++
	snapshot.Instances[0].ID = "new-external-id"
	state = importSnapshot()
	deny()
	if len(state.RemovedPlacements) != 1 || state.Placements["new-external-id"].ModelID != nil {
		t.Fatal("external ID change lost history or transferred mapping")
	}
	archive := state.RemovedPlacements[0]
	path := "/api/resource/removed-placements/" + strconv.FormatInt(archive.ID, 10)
	if response := doRequest(env.base, "DELETE", path, env.apiKey, map[string]any{"expected_revision": state.Revision, "confirm": true}); response.Status < 400 {
		t.Fatal("customer cleared history")
	}
	if response := doRequest(env.base, "DELETE", path, env.session, map[string]any{"expected_revision": state.Revision, "confirm": false}); response.Status != 400 {
		t.Fatalf("unconfirmed cleanup: %d %s", response.Status, response.Body)
	}
	if response := doRequest(env.base, "DELETE", path, env.session, map[string]any{"expected_revision": state.Revision - 1, "confirm": true}); response.Status != 409 {
		t.Fatal("stale cleanup")
	}
	state = publicationAPIData[model.ResourceCatalog](t, doRequest(env.base, "DELETE", path, env.session, map[string]any{"expected_revision": state.Revision, "confirm": true}))
	if len(state.RemovedPlacements) != 0 || len(state.Placements) != 1 {
		t.Fatal("cleanup changed live inventory")
	}
	var audits int64
	model.DB.Model(&model.AuditLog{}).Where("path = ? AND method = ? AND status = 200", path, "DELETE").Count(&audits)
	if audits != 1 {
		t.Fatal("cleanup audit missing", audits)
	}
}

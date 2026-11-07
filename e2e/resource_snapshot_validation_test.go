package main

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strconv"
	"testing"

	"starhub/model"
)

func TestResourceSnapshotInvalidNumbersPreservePublication(t *testing.T) {
	env := setupRelayApp(t)
	publicationAPIData[any](t, doRequest(env.base, "PUT", "/api/service-group", env.session, map[string]any{"group_id": "default", "name": "Default", "enabled": true, "visibility": "common", "multiplier": "1"}))
	channelID := createChannelID(t, env, map[string]any{"name": "Snapshot validation", "type": 1, "key": "synthetic-validation-key", "base_url": "https://example.invalid", "models": "alpha", "group": "default", "status": 1})
	snapshot := model.ResourceSnapshot{
		ID: "numeric-validation", Sequence: 1, PublishedAt: "2026-11-02T00:00:00Z",
		Channels:  []model.ResourceChannel{{ID: "supplier", Name: "Supplier", Enabled: true}},
		Instances: []model.ResourceInstance{{ID: "resource", ChannelID: "supplier", UpstreamModel: "alpha", CanonicalModel: "alpha", Protocols: []string{"OpenAI"}, Capabilities: []string{"text"}, ContextLength: 128000, Status: "available", LatencyMS: 12}},
	}
	catalog := publicationAPIData[model.ResourceCatalog](t, doRequest(env.base, "PUT", "/api/resource", env.session, map[string]any{"expected_revision": 0, "snapshot": snapshot}))
	name := "alpha"
	publicationAPIData[model.ResourceCatalog](t, doRequest(env.base, "PUT", "/api/placement/resource", env.session, map[string]any{"expected_revision": catalog.Revision, "placement": model.ResourcePlacement{ModelID: &name, GroupIDs: []string{"default"}, Enabled: true}}))
	publicationContext := publicationAPIData[model.ResourcePublicationContext](t, doRequest(env.base, "GET", "/api/resource/publication-context", env.session, nil))
	publish := map[string]any{
		"expected_resource_revision": publicationContext.ResourceRevision,
		"expected_channel_revision":  publicationContext.ChannelRevision,
		"expected_option_version":    publicationContext.OptionVersion,
		"bindings":                   []model.ResourceChannelBinding{{SourceChannelID: "supplier", GatewayChannelID: strconv.FormatInt(channelID, 10)}},
	}
	plan := publicationAPIData[model.ResourcePublicationPreview](t, doRequest(env.base, "POST", "/api/resource/publication-preview", env.session, publish))
	if !plan.Ready || len(plan.ProposedRoutes) != 1 {
		t.Fatalf("publication fixture not ready: %+v", plan)
	}
	publish["review_digest"] = plan.ReviewDigest
	publicationAPIData[model.ResourcePublicationAcknowledgement](t, doRequest(env.base, "PUT", "/api/resource/publication", env.session, publish))
	before := publicationAPIData[model.ResourceCatalog](t, doRequest(env.base, "GET", "/api/resource", env.session, nil))
	bindingsBefore := publicationAPIData[model.ResourceBindingState](t, doRequest(env.base, "GET", "/api/resource/publication-state", env.session, nil))
	if len(bindingsBefore.Bindings) != 1 || bindingsBefore.Bindings[0].RouteEligiblePairCount != 1 {
		t.Fatalf("fixture has no published route: %+v", bindingsBefore)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}

	invalidCases := map[string]func(map[string]any){
		"sequence-replay":       func(s map[string]any) { s["sequence"] = 1 },
		"sequence-fraction":     func(s map[string]any) { s["sequence"] = 1.5 },
		"duplicate-resource":    func(s map[string]any) { rows := s["instances"].([]any); s["instances"] = append(rows, rows[0]) },
		"duplicate-channel":     func(s map[string]any) { rows := s["channels"].([]any); s["channels"] = append(rows, rows[0]) },
		"null-channel-state":    func(s map[string]any) { s["channels"].([]any)[0].(map[string]any)["enabled"] = nil },
		"missing-channel-state": func(s map[string]any) { delete(s["channels"].([]any)[0].(map[string]any), "enabled") },
		"invalid-channel-state": func(s map[string]any) { s["channels"].([]any)[0].(map[string]any)["enabled"] = "false" },
		"channel-reference":     func(s map[string]any) { s["instances"].([]any)[0].(map[string]any)["channelId"] = "missing" },
		"protocol":              func(s map[string]any) { s["instances"].([]any)[0].(map[string]any)["protocols"] = []string{"invalid"} },
		"capability": func(s map[string]any) {
			s["instances"].([]any)[0].(map[string]any)["capabilities"] = []string{"invalid"}
		},
		"status":             func(s map[string]any) { s["instances"].([]any)[0].(map[string]any)["status"] = "invalid" },
		"null-score":         func(s map[string]any) { s["instances"].([]any)[0].(map[string]any)["score"] = nil },
		"bad-date":           func(s map[string]any) { s["publishedAt"] = "2026-02-31T00:00:00Z" },
		"injected-placement": func(s map[string]any) { s["placements"] = map[string]any{"resource": map[string]any{"enabled": true}} },
	}
	for name, mutate := range invalidCases {
		t.Run(name, func(t *testing.T) {
			var next map[string]any
			if err := json.Unmarshal(encoded, &next); err != nil {
				t.Fatal(err)
			}
			next["sequence"] = 2
			next["instances"].([]any)[0].(map[string]any)["upstreamModel"] = "changed-upstream"
			mutate(next)
			response := doRequest(env.base, "PUT", "/api/resource", env.session, map[string]any{"expected_revision": before.Revision, "snapshot": next})
			if response.Status != 400 && response.Status != 409 {
				t.Fatalf("invalid import accepted: %d %s", response.Status, response.Body)
			}
			after := publicationAPIData[model.ResourceCatalog](t, doRequest(env.base, "GET", "/api/resource", env.session, nil))
			bindings := publicationAPIData[model.ResourceBindingState](t, doRequest(env.base, "GET", "/api/resource/publication-state", env.session, nil))
			if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(bindingsBefore, bindings) {
				t.Fatal("invalid import changed published state")
			}
		})
	}
	for _, field := range []string{"contextLength", "latencyMs"} {
		for _, test := range []struct {
			name  string
			value any
		}{
			{"null", nil}, {"missing", nil}, {"string", "0"}, {"boolean", false},
			{"object", map[string]any{}}, {"array", []any{}},
			{"negative", -1}, {"unsafe", int64(1 << 53)},
		} {
			t.Run(field+"/"+test.name, func(t *testing.T) {
				var next map[string]any
				if err := json.Unmarshal(encoded, &next); err != nil {
					t.Fatal(err)
				}
				next["sequence"] = snapshot.Sequence + 1
				instance := next["instances"].([]any)[0].(map[string]any)
				// A bad import must not revoke the existing route or disable its placement.
				instance["upstreamModel"] = "changed-upstream"
				if test.name == "missing" {
					delete(instance, field)
				} else {
					instance[field] = test.value
				}
				response := doRequest(env.base, "PUT", "/api/resource", env.session, map[string]any{"expected_revision": before.Revision, "snapshot": next})
				if response.Status != http.StatusBadRequest {
					t.Errorf("invalid numeric field accepted: %d %s", response.Status, response.Body)
				}
				after := publicationAPIData[model.ResourceCatalog](t, doRequest(env.base, "GET", "/api/resource", env.session, nil))
				bindingsAfter := publicationAPIData[model.ResourceBindingState](t, doRequest(env.base, "GET", "/api/resource/publication-state", env.session, nil))
				if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(bindingsBefore, bindingsAfter) {
					t.Fatal("invalid import changed the snapshot, placements, revisions or published routes")
				}
			})
		}
	}
	// Explicit zero values remain valid source metadata, including unknown context or latency.
	snapshot.Sequence++
	snapshot.Instances[0].ContextLength = 0
	snapshot.Instances[0].LatencyMS = 0
	after := publicationAPIData[model.ResourceCatalog](t, doRequest(env.base, "PUT", "/api/resource", env.session, map[string]any{"expected_revision": before.Revision, "snapshot": snapshot}))
	if after.Revision != before.Revision+1 || after.Snapshot.Instances[0].ContextLength != 0 || after.Snapshot.Instances[0].LatencyMS != 0 {
		t.Fatalf("explicit zero metadata was not saved: %+v", after)
	}
}

package main

import (
	"encoding/json"
	"testing"

	"starhub/model"
)

func TestGroupRevisionGuardsActualRouteAndRecreatedIdentity(t *testing.T) {
	env := setupRelayApp(t)
	type groupReply struct {
		ID       int64 `json:"id"`
		Revision int64 `json:"revision"`
	}
	save := func(body map[string]any) groupReply {
		t.Helper()
		response := doRequest(env.base, "PUT", "/api/service-group", env.session, body)
		if response.Status != 200 {
			t.Fatalf("save: %d %s", response.Status, response.Body)
		}
		var reply struct {
			Data groupReply `json:"data"`
		}
		if err := json.Unmarshal([]byte(response.Body), &reply); err != nil {
			t.Fatal(err)
		}
		return reply.Data
	}
	payload := map[string]any{"group_id": "guarded", "name": "Original", "multiplier": "0.8", "expected_group_id": 0, "expected_revision": 0}
	first := save(payload)
	if first.ID < 1 || first.Revision != 1 {
		t.Fatal("guarded create did not return stored identity/revision")
	}
	version, err := model.OptionVersion()
	if err != nil {
		t.Fatal(err)
	}
	delete(payload, "expected_group_id")
	delete(payload, "expected_revision")
	payload["name"] = "Legacy winner"
	current := save(payload)
	afterVersion, err := model.OptionVersion()
	if err != nil || afterVersion != version || current.Revision != 2 {
		t.Fatal("legacy metadata-only edit did not advance its own revision")
	}
	payload["name"] = "Stale"
	payload["multiplier"] = "0.2"
	payload["expected_group_id"] = first.ID
	payload["expected_revision"] = first.Revision
	if response := doRequest(env.base, "PUT", "/api/service-group", env.session, payload); response.Status != 409 {
		t.Fatalf("stale save: %d %s", response.Status, response.Body)
	}
	remove := map[string]any{"expected_group_id": first.ID, "expected_revision": first.Revision}
	if response := doRequest(env.base, "DELETE", "/api/service-group/guarded", env.session, remove); response.Status != 409 {
		t.Fatalf("stale delete: %d %s", response.Status, response.Body)
	}
	remove["expected_revision"] = current.Revision
	if response := doRequest(env.base, "DELETE", "/api/service-group/guarded", env.session, remove); response.Status != 200 {
		t.Fatalf("current delete: %d %s", response.Status, response.Body)
	}
	newGroup := save(map[string]any{"group_id": "guarded", "name": "Recreated", "multiplier": "0.7", "expected_group_id": 0, "expected_revision": 0})
	if newGroup.ID == first.ID || newGroup.Revision != first.Revision {
		t.Fatal("recreated fixture identity not distinct")
	}
	if response := doRequest(env.base, "PUT", "/api/service-group", env.session, payload); response.Status != 409 {
		t.Fatalf("old identity overwrote recreation: %d %s", response.Status, response.Body)
	}
	stored, err := model.GetServiceGroup("guarded")
	if err != nil || stored.Name != "Recreated" || stored.Multiplier != "0.7" {
		t.Fatal("stale route changed recreated state")
	}
}

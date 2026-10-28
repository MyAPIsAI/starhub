package controller

import (
	"fmt"
	"testing"

	"starhub/model"
)

func TestGroupRequestGuardsMetadataOnlyEditsAndDelete(t *testing.T) {
	newTestDB(t)
	put := func(body string) map[string]any {
		t.Helper()
		response := call(UpsertServiceGroup, testRequest{method: "PUT", target: "/api/service-group", body: body})
		var data map[string]any
		decodeData(t, response, &data)
		return data
	}
	first := put(`{"group_id":"guarded","name":"Original","multiplier":"0.8","expected_group_id":0,"expected_revision":0}`)
	id := int64(first["id"].(float64))
	if first["revision"] != float64(1) {
		t.Fatal("initial metadata revision missing")
	}
	version, err := model.OptionVersion()
	if err != nil {
		t.Fatal(err)
	}
	put(`{"group_id":"guarded","name":"Winner","multiplier":"0.8"}`)
	afterVersion, err := model.OptionVersion()
	if err != nil || version != afterVersion {
		t.Fatal("metadata-only fixture changed pricing version")
	}
	body := fmt.Sprintf(`{"group_id":"guarded","name":"Stale","multiplier":"0.2","expected_group_id":%d,"expected_revision":1}`, id)
	expectStatus(t, call(UpsertServiceGroup, testRequest{method: "PUT", target: "/api/service-group", body: body}), 409)
	current, err := model.GetServiceGroup("guarded")
	if err != nil || current.Name != "Winner" || current.Revision != 2 || current.Multiplier != "0.8" {
		t.Fatal("stale request overwrote metadata or price")
	}
	body = fmt.Sprintf(`{"group_id":"guarded","name":"Saved","multiplier":"0.6","expected_group_id":%d,"expected_revision":2}`, id)
	last := put(body)
	if last["revision"] != float64(3) {
		t.Fatal("successful edit did not advance metadata revision")
	}
	remove := func(revision int64) testRequest {
		return testRequest{method: "DELETE", target: "/api/service-group/guarded", path: map[string]string{"id": "guarded"}, body: fmt.Sprintf(`{"expected_group_id":%d,"expected_revision":%d}`, id, revision)}
	}
	expectStatus(t, call(DeleteServiceGroup, remove(2)), 409)
	expectStatus(t, call(DeleteServiceGroup, remove(3)), 200)
}

func TestGroupRequestRejectsMalformedOrPartialPreconditions(t *testing.T) {
	newTestDB(t)
	for _, fields := range []string{`"expected_group_id":0`, `"expected_revision":0`, `"expected_group_id":null,"expected_revision":null`, `"expected_group_id":0,"expected_revision":null`, `"expected_group_id":0,"expected_revision":0.5`, `"expected_group_id":-1,"expected_revision":0`, `"expected_group_id":0,"expected_revision":1`, `"expected_group_id":9007199254740992,"expected_revision":0`} {
		body := `{"group_id":"guarded","name":"Invalid","multiplier":"0.8",` + fields + `}`
		expectStatus(t, call(UpsertServiceGroup, testRequest{method: "PUT", target: "/api/service-group", body: body}), 400)
	}
	for _, body := range []string{`{}`, `{"expected_group_id":1}`, `{"expected_group_id":1,"expected_revision":null}`, `{"expected_group_id":1,"expected_revision":0,"unknown":true}`} {
		expectStatus(t, call(DeleteServiceGroup, testRequest{method: "DELETE", target: "/api/service-group/guarded", path: map[string]string{"id": "guarded"}, body: body}), 400)
	}
	groups, err := model.ListServiceGroups()
	if err != nil || len(groups) != 0 {
		t.Fatal("invalid guards persisted metadata")
	}
}

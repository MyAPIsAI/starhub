package controller

import (
	"fmt"
	"testing"

	"starhub/model"
	"starhub/setting"
)

func TestGroupCatalogAndCombinedGuardRequests(t *testing.T) {
	newTestDB(t)
	var catalog struct {
		Groups         []any              `json:"groups"`
		Version        int64              `json:"option_version"`
		RevisionGuards bool               `json:"revision_guards"`
		OptionGuard    bool               `json:"option_version_guard"`
		AccountGuard   bool               `json:"account_access_guard"`
		AccountLists   bool               `json:"account_list_validation"`
		Ratios         map[string]float64 `json:"group_ratios"`
	}
	decodeData(t, call(GetServiceGroupCatalog, testRequest{method: "GET", target: "/api/service-group/configuration"}), &catalog)
	if catalog.Groups == nil || len(catalog.Groups) != 0 || !catalog.RevisionGuards || !catalog.OptionGuard || !catalog.AccountGuard || !catalog.AccountLists || catalog.Ratios["default"] != 1 {
		t.Fatal("empty catalog lacks explicit supported guards")
	}
	for _, fields := range []string{`"expected_option_version":0`, `"expected_group_id":0,"expected_revision":0,"expected_option_version":null`, `"expected_group_id":0,"expected_revision":0,"expected_option_version":0.5`, `"expected_group_id":0,"expected_revision":0,"expected_option_version":-1`} {
		expectStatus(t, call(UpsertServiceGroup, testRequest{method: "PUT", target: "/api/service-group", body: `{"group_id":"guarded","name":"Invalid","multiplier":"0.8",` + fields + `}`}), 400)
	}
	var group struct {
		ID       int64 `json:"id"`
		Revision int64 `json:"revision"`
	}
	decodeData(t, call(UpsertServiceGroup, testRequest{method: "PUT", target: "/api/service-group", body: fmt.Sprintf(`{"group_id":"guarded","name":"Original","multiplier":"0.8","expected_group_id":0,"expected_revision":0,"expected_option_version":%d}`, catalog.Version)}), &group)
	decodeData(t, call(GetServiceGroupCatalog, testRequest{method: "GET", target: "/api/service-group/configuration"}), &catalog)
	if _, err := model.SaveOption(setting.OptionKeyModelRatio, `{"gpt-4o":99}`, 1); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"group_id":"guarded","name":"Stale","multiplier":"0.6","expected_group_id":%d,"expected_revision":%d,"expected_option_version":%d}`, group.ID, group.Revision, catalog.Version)
	expectStatus(t, call(UpsertServiceGroup, testRequest{method: "PUT", target: "/api/service-group", body: body}), 409)
	body = fmt.Sprintf(`{"expected_group_id":%d,"expected_revision":%d,"expected_option_version":%d}`, group.ID, group.Revision, catalog.Version)
	expectStatus(t, call(DeleteServiceGroup, testRequest{method: "DELETE", target: "/api/service-group/guarded", path: map[string]string{"id": "guarded"}, body: body}), 409)
	for _, version := range []string{"null", "-1", "0.5"} {
		body = fmt.Sprintf(`{"expected_group_id":%d,"expected_revision":%d,"expected_option_version":%s}`, group.ID, group.Revision, version)
		expectStatus(t, call(DeleteServiceGroup, testRequest{method: "DELETE", target: "/api/service-group/guarded", path: map[string]string{"id": "guarded"}, body: body}), 400)
	}
}

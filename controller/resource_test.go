package controller

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"starhub/model"
)

func TestResourceRequestValidation(t *testing.T) {
	newTestDB(t)
	for _, body := range []string{`{}`, `null`, `{"expected_revision":0,"snapshot":null}`, `{"expected_revision":0,"snapshot":{},"key":"secret"}`, `{} {}`, `{"expected_revision":0.5,"snapshot":{}}`, `{"expected_revision":0,"snapshot":{"id":"x","key":"secret"}}`} {
		rr := call(UpdateResources, testRequest{method: "PUT", target: "/api/resource", body: body})
		expectStatus(t, rr, http.StatusBadRequest)
	}
	large := `{"expected_revision":0,"snapshot":{"source":"` + strings.Repeat("x", (1<<20)+4096) + `"}}`
	expectStatus(t, call(UpdateResources, testRequest{method: "PUT", target: "/api/resource", body: large}), http.StatusRequestEntityTooLarge)
	for _, body := range []string{`{}`, `{"expected_revision":0,"placement":null}`, `{"expected_revision":0,"placement":{"modelId":null,"groupIds":[],"enabled":false,"key":"secret"}}`} {
		expectStatus(t, call(UpdatePlacement, testRequest{method: "PUT", target: "/api/placement/item", path: map[string]string{"id": "item"}, body: body}), http.StatusBadRequest)
	}
	state, err := model.ReadResourceCatalog(context.Background())
	if err != nil || state.Revision != 0 || state.Snapshot != nil {
		t.Fatalf("invalid body changed catalog: %+v %v", state, err)
	}
}

func TestResourceReadFailureDoesNotExposeDatabaseDetails(t *testing.T) {
	newTestDB(t)
	if err := model.DB.Migrator().DropTable(&model.ResourceCatalogRecord{}); err != nil {
		t.Fatal(err)
	}
	rr := call(ListResources, testRequest{target: "/api/resource"})
	expectError(t, rr, http.StatusInternalServerError, "resource catalog is unavailable")
	if strings.Contains(rr.Body.String(), "resource_catalog_records") {
		t.Fatal("database detail exposed")
	}
	expectError(t, call(GetResourceBindingState, testRequest{target: "/api/resource/publication-state"}), http.StatusInternalServerError, "resource catalog is unavailable")
}

func TestResourceBindingStateStrictPagination(t *testing.T) {
	for _, query := range []string{"page=0", "page=-1", "page=1000001", "page=01", "page=1.5", "page=", "page=1&page=2", "page_size=0", "page_size=101", "page_size=abc", "key=secret"} {
		expectStatus(t, call(GetResourceBindingState, testRequest{target: "/api/resource/publication-state?" + query}), http.StatusBadRequest)
	}
}

func TestResourceBindingChangeStrictBodyAndReceiptIdentifiers(t *testing.T) {
	newTestDB(t)
	for _, body := range []string{`{}`, `null`, `{} {}`, `{"expected_resource_revision":0,"operation":"release"}`, `{"expected_resource_revision":0,"expected_channel_revision":0,"expected_source_channel_id":"source","expected_publication_id":1,"operation":"revoke","request_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`, `{"expected_resource_revision":0,"expected_channel_revision":0,"expected_source_channel_id":"source","expected_publication_id":"0","operation":"delete","request_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`, `{"expected_resource_revision":0,"expected_channel_revision":0,"actor_id":1}`} {
		expectStatus(t, call(ChangeResourceBinding, testRequest{method: "PUT", target: "/api/resource/bindings/1", path: map[string]string{"id": "1"}, body: body}), http.StatusBadRequest)
	}
	expectStatus(t, call(GetResourceBindingChange, testRequest{target: "/api/resource/binding-changes/bad", path: map[string]string{"request_id": "bad"}}), http.StatusBadRequest)
	expectStatus(t, call(GetResourceBindingChange, testRequest{target: "/api/resource/binding-changes/" + strings.Repeat("a", 32), path: map[string]string{"request_id": strings.Repeat("a", 32)}}), http.StatusNotFound)
}

func TestResourcePublicationBodyRequiresAllVersionsAndStrictBindings(t *testing.T) {
	newTestDB(t)
	for _, body := range []string{
		`{}`, `null`,
		`{"expected_resource_revision":0,"expected_channel_revision":0,"bindings":[]}`,
		`{"expected_resource_revision":0,"expected_channel_revision":0,"expected_option_version":0,"bindings":null}`,
		`{"expected_resource_revision":0,"expected_channel_revision":0,"expected_option_version":0,"bindings":[{"source_channel_id":"source","gateway_channel_id":1}]}`,
		`{"expected_resource_revision":0,"expected_channel_revision":0,"expected_option_version":0,"bindings":[{"source_channel_id":"source","gateway_channel_id":"1","key":"secret"}]}`,
		`{"expected_resource_revision":0,"expected_channel_revision":0,"expected_option_version":0,"bindings":[{"source_channel_id":"source","gateway_channel_id":"1"}]} {}`,
	} {
		expectStatus(t, call(PreviewResourcePublication, testRequest{method: "POST", target: "/api/resource/publication-preview", body: body}), http.StatusBadRequest)
	}
}

func TestResourcePublishBodyRejectsMissingGuardsAndClientSuppliedFacts(t *testing.T) {
	newTestDB(t)
	for _, body := range []string{`{}`, `null`, `{"expected_resource_revision":0,"expected_channel_revision":0,"bindings":[]}`, `{"expected_resource_revision":0,"expected_channel_revision":0,"expected_option_version":0,"bindings":[],"review_digest":"bad"}`, `{"expected_resource_revision":0,"expected_channel_revision":0,"expected_option_version":0,"review_digest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","bindings":[{"source_channel_id":"source","gateway_channel_id":1}]}`, `{"expected_resource_revision":0,"expected_channel_revision":0,"expected_option_version":0,"review_digest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","bindings":[],"actor_id":1}`, `{"expected_resource_revision":0,"expected_channel_revision":0,"expected_option_version":0,"review_digest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","bindings":[],"proposed_routes":[]}`} {
		expectStatus(t, call(PublishResourceGateway, testRequest{method: "PUT", target: "/api/resource/publication", body: body}), http.StatusBadRequest)
	}
	expectStatus(t, call(GetResourcePublicationReceipt, testRequest{target: "/api/resource/publication-receipt/bad", path: map[string]string{"digest": "bad"}}), http.StatusBadRequest)
	expectStatus(t, call(GetResourcePublicationReceipt, testRequest{target: "/api/resource/publication-receipt/" + strings.Repeat("0", 64), path: map[string]string{"digest": strings.Repeat("0", 64)}}), http.StatusNotFound)
}

package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"starhub/common"
	"starhub/model"
)

func TestRegionVisibilityMatchesCatalogAndRelayListings(t *testing.T) {
	newTestDB(t)
	t.Setenv("STARHUB_REGION_PROFILE", "visibility")
	profile := &model.RegionProfile{ID: "visibility", Name: "Visibility", AllowedModels: "region-allowed", AllowedVendors: "accepted", Timezone: "Asia/Dubai", Enabled: true}
	if err := model.SaveRegionProfile(profile, -1); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"region-allowed", "region-hidden"} {
		if err := model.InsertChannel(&model.Channel{Name: name, Models: name, Group: "default", Status: 1}); err != nil {
			t.Fatal(err)
		}
		if err := model.UpsertModelMeta(&model.ModelMeta{Model: name, Vendor: "accepted", Tags: "text"}); err != nil {
			t.Fatal(err)
		}
	}
	for _, handler := range []http.HandlerFunc{PlazaListModels, ListModels} {
		r := httptest.NewRequest("GET", "/api/plaza/models", nil)
		r = r.WithContext(common.WithValues(r.Context(), common.CtxUsingGroup, "default"))
		w := httptest.NewRecorder()
		handler(w, r)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "region-allowed") || strings.Contains(w.Body.String(), "region-hidden") {
			t.Fatalf("regional listing mismatch: %d %s", w.Code, w.Body.String())
		}
	}
	w := call(ActiveRegionPublicHandler, testRequest{target: "/api/public/region"})
	if w.Code != 200 || strings.Contains(w.Body.String(), "region-allowed") || strings.Contains(w.Body.String(), "region-hidden") || !strings.Contains(w.Body.String(), "Asia/Dubai") {
		t.Fatal("public region response leaked model policy or omitted locale")
	}
}

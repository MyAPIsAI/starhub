package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"starhub/common"
	"starhub/model"
	"starhub/relay/dto"
)

func TestDeploymentRegionBlocksExplicitAndSmartCandidatesBeforeAdmission(t *testing.T) {
	setupAutoRoutingDB(t)
	t.Setenv("STARHUB_REGION_PROFILE", "restricted")
	profile := &model.RegionProfile{ID: "restricted", Name: "Restricted", AllowedModels: "allowed", AllowedVendors: "accepted", FeatureFlags: "{}", Enabled: true}
	if err := model.SaveRegionProfile(profile, -1); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"allowed", "blocked"} {
		if err := model.InsertChannel(&model.Channel{Name: name, Models: name, Group: "default", Status: 1, Weight: 1}); err != nil {
			t.Fatal(err)
		}
		if err := model.UpsertModelMeta(&model.ModelMeta{Model: name, Vendor: "accepted", Tags: "text"}); err != nil {
			t.Fatal(err)
		}
	}
	call := func(name string, want int) {
		t.Helper()
		r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"`+name+`","messages":[{"role":"user","content":"test"}]}`))
		r.Header.Set("Content-Type", "application/json")
		r = r.WithContext(common.WithValues(r.Context(), common.CtxUsingGroup, "default", common.CtxTokenID, int64(1)))
		w := httptest.NewRecorder()
		Distribute(dto.InboundOpenAI)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got, _ := r.Context().Value(common.CtxRequestModel).(string); got == "blocked" {
				t.Fatal("region-excluded smart candidate was selected")
			}
			w.WriteHeader(204)
		})).ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("model %s: %d %s", name, w.Code, w.Body.String())
		}
	}
	call("blocked", 403)
	call("allowed", 204)
	call("myapis-auto", 204)
	if err := model.UpsertModelMeta(&model.ModelMeta{Model: "allowed", Vendor: "excluded", Tags: "text"}); err != nil {
		t.Fatal(err)
	}
	call("allowed", 403)
	profile.FeatureFlags = `{"relay":false}`
	if err := model.SaveRegionProfile(profile, profile.Revision); err != nil {
		t.Fatal(err)
	}
	call("allowed", 403)
}

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

func TestUnavailableRegionRejectsModelAdmissionWithoutFinancialEffects(t *testing.T) {
	setupAutoRoutingDB(t)
	user := &model.User{Username: "region-admission", Quota: 1000, Status: 1, AffCode: common.GenerateAffCode()}
	if err := model.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	token := &model.Token{UserID: user.ID, Name: "region-admission", UnlimitedQuota: true, Status: 1}
	if err := model.InsertToken(token); err != nil {
		t.Fatal(err)
	}
	if _, err := model.ReadMoneyAccount(user.ID); err != nil {
		t.Fatal(err)
	}
	profile := &model.RegionProfile{ID: "disabled-region", Name: "Disabled", Enabled: true}
	if err := model.SaveRegionProfile(profile, -1); err != nil {
		t.Fatal(err)
	}
	if err := model.DB.Model(profile).UpdateColumn("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	for _, region := range []string{"missing-region", "disabled-region"} {
		t.Run(region, func(t *testing.T) {
			t.Setenv("STARHUB_REGION_PROFILE", region)
			r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"allowed","messages":[{"role":"user","content":"fixture"}]}`))
			r.Header.Set("Content-Type", "application/json")
			r = r.WithContext(common.WithValues(r.Context(), common.CtxUserID, user.ID, common.CtxTokenID, token.ID, common.CtxUsingGroup, "default"))
			called := false
			response := httptest.NewRecorder()
			Distribute(dto.InboundOpenAI)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })).ServeHTTP(response, r)
			if response.Code != 503 || called {
				t.Fatalf("unavailable policy admitted consumption: %d %s", response.Code, response.Body.String())
			}
			after, err := model.GetUserByID(user.ID)
			if err != nil || after.Quota != 1000 || after.UsedQuota != 0 {
				t.Fatal("failed admission changed balances", err)
			}
			var receipts int64
			if err := model.DB.Model(&model.QuotaReservation{}).Where("user_id = ?", user.ID).Count(&receipts).Error; err != nil || receipts != 0 {
				t.Fatal("failed admission created a hold", err)
			}
			if err := model.VerifyMoneyLedger(user.ID); err != nil {
				t.Fatal(err)
			}
		})
	}
}

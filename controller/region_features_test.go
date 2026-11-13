package controller

import (
	"net/http"
	"testing"

	"starhub/model"
)

func TestRegionFeatureFlagsRejectWritesBeforeSideEffects(t *testing.T) {
	newTestDB(t)
	t.Setenv("STARHUB_REGION_PROFILE", "features")
	profile := &model.RegionProfile{ID: "features", Name: "Features", FeatureFlags: `{"registration":false,"sms":false,"email":false,"payments":false}`, Enabled: true}
	if err := model.SaveRegionProfile(profile, -1); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		handler    http.HandlerFunc
		path, body string
	}{{Register, "/api/user/register", `{"username":"region-user","password":"RegionPassword93"}`}, {SendPhoneVerification, "/api/user/phone-verification", `{"phone":"13812345678"}`}, {SendEmailVerificationHandler, "/api/user/email-verification", `{"email":"user@example.test"}`}, {CreateRechargeOrder, "/api/recharge", `{"amount_usd":"10","channel":"card"}`}} {
		w := call(tc.handler, testRequest{method: "POST", target: tc.path, body: tc.body})
		expectStatus(t, w, 403)
	}
	if _, err := model.GetUserByUsername("region-user"); err != model.ErrUserNotFound {
		t.Fatal("disabled registration created an account")
	}
	var orders int64
	if err := model.DB.Model(&model.RechargeOrder{}).Count(&orders).Error; err != nil || orders != 0 {
		t.Fatal("disabled payment created an order")
	}
}

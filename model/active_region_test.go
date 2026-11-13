package model

import "testing"

func TestActiveRegionSelectionAndPolicyFailClosed(t *testing.T) {
	newTestDB(t)
	t.Setenv("STARHUB_REGION_PROFILE", "")
	legacy, err := ActiveRegionProfile()
	if err != nil || legacy.Timezone != "UTC" || !legacy.FeatureEnabled("relay") {
		t.Fatal("legacy deployment defaults changed")
	}
	profile := &RegionProfile{ID: "arabic", Name: "Arabic deployment", Timezone: "Asia/Dubai", DefaultLanguage: "ar", SupportedLanguages: "ar,en", FeatureFlags: `{"relay":false,"payments":false}`, AuthPlugins: "password,email", Enabled: true}
	if err := SaveRegionProfile(profile, -1); err != nil {
		t.Fatal(err)
	}
	t.Setenv("STARHUB_REGION_PROFILE", profile.ID)
	active, err := ActiveRegionProfile()
	if err != nil || active.DefaultLanguage != "ar" || active.FeatureEnabled("relay") || active.PaymentEnabled("mock") || active.AuthEnabled("sms") || !active.AuthEnabled("password") {
		t.Fatal("active profile was not enforced")
	}
	if err := DeleteRegionProfile(profile.ID); err == nil {
		t.Fatal("active profile was deleted")
	}
	profile.Enabled = false
	if err := SaveRegionProfile(profile, profile.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := ActiveRegionProfile(); err != ErrRegionUnavailable {
		t.Fatal("disabled profile silently fell back")
	}
	t.Setenv("STARHUB_REGION_PROFILE", "missing")
	if _, err := ActiveRegionProfile(); err != ErrRegionUnavailable {
		t.Fatal("explicit missing region silently fell back")
	}
}

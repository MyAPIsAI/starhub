package model

import (
	"testing"

	"starhub/common"
)

func TestSaveAndGetRegionProfile(t *testing.T) {
	newTestDB(t)
	profile := &RegionProfile{
		ID:                 "cn",
		Name:               "China",
		DefaultLanguage:    "zh",
		SupportedLanguages: "zh,en",
		Currency:           "CNY",
		Timezone:           "Asia/Shanghai",
		CountryCode:        "CN",
		LegalVersion:       "v1.0",
		FeatureFlags:       `{"sms":true}`,
		Enabled:            true,
	}
	if err := SaveRegionProfile(profile, -1); err != nil {
		t.Fatalf("SaveRegionProfile failed: %v", err)
	}
	if profile.Revision != 0 {
		t.Fatalf("initial revision should be 0, got %d", profile.Revision)
	}

	got, err := GetRegionProfile("cn")
	if err != nil {
		t.Fatalf("GetRegionProfile failed: %v", err)
	}
	if got.DefaultLanguage != "zh" || got.Currency != "CNY" || got.Timezone != "Asia/Shanghai" {
		t.Fatalf("unexpected profile: %+v", got)
	}
}

func TestSaveRegionProfileUpdateWithRevision(t *testing.T) {
	newTestDB(t)
	profile := &RegionProfile{
		ID: "me", Name: "Middle East",
		DefaultLanguage:    "ar",
		SupportedLanguages: "ar,en",
		Currency:           "AED",
		Timezone:           "Asia/Dubai",
	}
	if err := SaveRegionProfile(profile, -1); err != nil {
		t.Fatal(err)
	}

	// Update with correct revision.
	profile.Name = "Middle East & North Africa"
	if err := SaveRegionProfile(profile, 0); err != nil {
		t.Fatalf("update with correct revision failed: %v", err)
	}
	if profile.Revision != 1 {
		t.Fatalf("revision should be 1, got %d", profile.Revision)
	}

	// Update with wrong revision.
	profile.Name = "Stale"
	if err := SaveRegionProfile(profile, 0); err == nil {
		t.Fatal("update with stale revision should fail")
	}
}

func TestSaveRegionProfileInvalidTimezone(t *testing.T) {
	newTestDB(t)
	profile := &RegionProfile{
		ID: "bad", Name: "Bad TZ",
		Timezone: "Mars/Colony",
	}
	if err := SaveRegionProfile(profile, -1); err == nil {
		t.Fatal("invalid timezone should be rejected")
	}
}

func TestListRegionProfiles(t *testing.T) {
	newTestDB(t)
	SaveRegionProfile(&RegionProfile{ID: "b", Name: "B", DefaultLanguage: "en"}, -1)
	SaveRegionProfile(&RegionProfile{ID: "a", Name: "A", DefaultLanguage: "zh"}, -1)

	list, err := ListRegionProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(list))
	}
	// Ordered by ID ASC
	if list[0].ID != "a" || list[1].ID != "b" {
		t.Fatalf("unexpected order: %s, %s", list[0].ID, list[1].ID)
	}
}

func TestDeleteRegionProfile(t *testing.T) {
	newTestDB(t)
	// Create two profiles so the "last enabled" guard doesn't fire.
	SaveRegionProfile(&RegionProfile{ID: "keep", Name: "Keep", DefaultLanguage: "en", Enabled: true}, -1)
	SaveRegionProfile(&RegionProfile{ID: "temp", Name: "Temp", DefaultLanguage: "en", Enabled: true}, -1)

	if err := DeleteRegionProfile("temp"); err != nil {
		t.Fatalf("DeleteRegionProfile failed: %v", err)
	}
	if _, err := GetRegionProfile("temp"); err != ErrRegionProfileNotFound {
		t.Fatalf("expected ErrRegionProfileNotFound, got %v", err)
	}
}

func TestDeleteLastEnabledRegionProfile(t *testing.T) {
	newTestDB(t)
	SaveRegionProfile(&RegionProfile{ID: "only", Name: "Only", DefaultLanguage: "en", Enabled: true}, -1)

	if err := DeleteRegionProfile("only"); err == nil {
		t.Fatal("should not delete the last enabled region profile")
	}
}

func TestRegionProfileAllowedChecks(t *testing.T) {
	r := &RegionProfile{
		AllowedVendors: "openai,anthropic",
		AllowedModels:  "gpt-4o,claude-sonnet-4",
	}
	if !r.IsVendorAllowed("openai") {
		t.Fatal("openai should be allowed")
	}
	if r.IsVendorAllowed("google") {
		t.Fatal("google should not be allowed")
	}
	if !r.IsModelAllowed("gpt-4o") {
		t.Fatal("gpt-4o should be allowed")
	}
	if r.IsModelAllowed("gemini-pro") {
		t.Fatal("gemini-pro should not be allowed")
	}

	// Empty = all allowed
	r2 := &RegionProfile{}
	if !r2.IsVendorAllowed("anything") || !r2.IsModelAllowed("anything") {
		t.Fatal("empty allowed lists should permit all")
	}
}

func TestGetRegionProfileNotFound(t *testing.T) {
	newTestDB(t)
	_, err := GetRegionProfile("nonexistent")
	if err != ErrRegionProfileNotFound {
		t.Fatalf("expected ErrRegionProfileNotFound, got %v", err)
	}
}

// Ensure common is used (for AffCode in test users).
var _ = common.GenerateAffCode

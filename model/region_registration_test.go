package model

import (
	"starhub/common"
	"testing"
)

func TestRegionalRegistrationPinsLegalVersionAndRollsBackFailedConsent(t *testing.T) {
	newTestDB(t)
	t.Setenv("STARHUB_REGION_PROFILE", "registration")
	profile := &RegionProfile{ID: "registration", Name: "Registration", LegalVersion: "v2", Enabled: true}
	if err := SaveRegionProfile(profile, -1); err != nil {
		t.Fatal(err)
	}
	user := &User{Username: "consenting", Status: 1, Role: 1, AffCode: common.GenerateAffCode()}
	if err := CreateRegisteredUser(user, "v1", profile); err != ErrRegionRegistrationChanged {
		t.Fatalf("stale consent accepted: %v", err)
	}
	if err := CreateRegisteredUser(user, "v2", profile); err != ErrRegionLegalUnavailable {
		t.Fatalf("unpublished legal pack accepted: %v", err)
	}
	for _, kind := range []string{"user_agreement", "privacy_policy"} {
		if err := PublishLegalDocument(&LegalDocument{Type: kind, Version: "v2", Language: "en", Content: "Published terms", RegionID: profile.ID}); err != nil {
			t.Fatal(err)
		}
	}
	if err := DB.Exec(`CREATE TRIGGER reject_registration_consent BEFORE INSERT ON user_agreements BEGIN SELECT RAISE(ABORT, 'consent failed'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if err := CreateRegisteredUser(user, "v2", profile); err == nil {
		t.Fatal("failed consent did not reject account")
	}
	if _, err := GetUserByUsername(user.Username); err != ErrUserNotFound {
		t.Fatal("account persisted without legal consent")
	}
	if err := DB.Exec("DROP TRIGGER reject_registration_consent").Error; err != nil {
		t.Fatal(err)
	}
	user.ID = 0
	if err := CreateRegisteredUser(user, "v2", profile); err != nil {
		t.Fatal(err)
	}
	consent, err := GetLatestAgreementConsent(user.ID)
	if err != nil || consent == nil || consent.AgreementVersion != "v2" {
		t.Fatal("accepted legal version not retained")
	}
}

func TestRegistrationRejectsChangedProfileEvenWhenLegalVersionIsUnchanged(t *testing.T) {
	newTestDB(t)
	t.Setenv("STARHUB_REGION_PROFILE", "registration-revision")
	profile := &RegionProfile{ID: "registration-revision", Name: "Registration", LegalVersion: "v1", Enabled: true}
	if err := SaveRegionProfile(profile, -1); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"user_agreement", "privacy_policy"} {
		if err := PublishLegalDocument(&LegalDocument{Type: kind, Version: "v1", Language: "en", Content: "Published fixture terms", RegionID: profile.ID}); err != nil {
			t.Fatal(err)
		}
	}
	accepted := *profile
	profile.FeatureFlags = `{"registration":false}`
	if err := SaveRegionProfile(profile, profile.Revision); err != nil {
		t.Fatal(err)
	}
	user := &User{Username: "stale-policy-consent", Status: 1, Role: 1, AffCode: common.GenerateAffCode()}
	if err := CreateRegisteredUser(user, "v1", &accepted); err != ErrRegionRegistrationChanged {
		t.Fatalf("stale policy accepted: %v", err)
	}
	for _, table := range []string{"users", "user_agreements"} {
		var count int64
		if err := DB.Table(table).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("partial %s record: %d %v", table, count, err)
		}
	}
}

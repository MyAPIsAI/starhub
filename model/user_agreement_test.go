package model

import (
	"testing"

	"starhub/common"
)

func TestRecordAndGetAgreementConsent(t *testing.T) {
	newTestDB(t)
	user := &User{Username: "agree-test", Password: "x", Role: 1, Status: 1, Quota: 0, Group: "default", AffCode: common.GenerateAffCode()}
	if err := CreateUser(user); err != nil {
		t.Fatal(err)
	}

	// No consent initially
	consent, err := GetLatestAgreementConsent(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if consent != nil {
		t.Fatal("expected nil consent before recording")
	}

	// Record consent
	if err := RecordAgreementConsent(nil, user.ID, "v1.0"); err != nil {
		t.Fatal(err)
	}

	consent, err = GetLatestAgreementConsent(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if consent == nil || consent.AgreementVersion != "v1.0" {
		t.Fatalf("expected v1.0 consent, got %+v", consent)
	}

	// Idempotent: recording same version again is a no-op
	if err := RecordAgreementConsent(nil, user.ID, "v1.0"); err != nil {
		t.Fatal(err)
	}
	var count int64
	DB.Model(&UserAgreement{}).Where("user_id = ?", user.ID).Count(&count)
	if count != 1 {
		t.Fatalf("expected 1 row, got %d", count)
	}

	// New version creates a new row
	if err := RecordAgreementConsent(nil, user.ID, "v1.1"); err != nil {
		t.Fatal(err)
	}
	consent, _ = GetLatestAgreementConsent(user.ID)
	if consent.AgreementVersion != "v1.1" {
		t.Fatalf("expected v1.1 as latest, got %s", consent.AgreementVersion)
	}
}

func TestRecordAgreementConsentEmptyVersion(t *testing.T) {
	newTestDB(t)
	user := &User{Username: "agree-empty", Password: "x", Role: 1, Status: 1, Quota: 0, Group: "default", AffCode: common.GenerateAffCode()}
	if err := CreateUser(user); err != nil {
		t.Fatal(err)
	}
	// Empty version is a no-op
	if err := RecordAgreementConsent(nil, user.ID, ""); err != nil {
		t.Fatal(err)
	}
	var count int64
	DB.Model(&UserAgreement{}).Where("user_id = ?", user.ID).Count(&count)
	if count != 0 {
		t.Fatalf("expected 0 rows for empty version, got %d", count)
	}
}

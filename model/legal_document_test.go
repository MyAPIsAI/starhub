package model

import (
	"testing"
)

func TestPublishAndGetLegalDocument(t *testing.T) {
	newTestDB(t)
	doc := &LegalDocument{
		Type:     "user_agreement",
		Version:  "v1.0",
		Language: "en",
		Content:  "Terms of Service v1.0",
	}
	if err := PublishLegalDocument(doc); err != nil {
		t.Fatalf("PublishLegalDocument failed: %v", err)
	}
	if doc.PublishedAt == 0 {
		t.Fatal("PublishedAt should be set")
	}

	got, err := GetLegalDocument("user_agreement", "en", "")
	if err != nil {
		t.Fatalf("GetLegalDocument failed: %v", err)
	}
	if got.Version != "v1.0" || got.Content != "Terms of Service v1.0" {
		t.Fatalf("unexpected document: %+v", got)
	}
}

func TestGetLegalDocumentLanguageFallback(t *testing.T) {
	newTestDB(t)
	// Publish only English version.
	PublishLegalDocument(&LegalDocument{
		Type: "privacy_policy", Version: "v2.0", Language: "en",
		Content: "Privacy Policy EN",
	})

	// Request Chinese — should fall back to English.
	got, err := GetLegalDocument("privacy_policy", "zh", "")
	if err != nil {
		t.Fatalf("expected fallback to English, got error: %v", err)
	}
	if got.Language != "en" {
		t.Fatalf("expected fallback language en, got %s", got.Language)
	}
}

func TestGetLegalDocumentRegionFallback(t *testing.T) {
	newTestDB(t)
	// Publish global version.
	PublishLegalDocument(&LegalDocument{
		Type: "user_agreement", Version: "v1.0", Language: "en",
		Content: "Global Terms",
	})

	// Request CN region — should fall back to global.
	got, err := GetLegalDocument("user_agreement", "en", "cn")
	if err != nil {
		t.Fatalf("expected fallback to global, got error: %v", err)
	}
	if got.RegionID != "" {
		t.Fatalf("expected global (empty region), got %s", got.RegionID)
	}
}

func TestPublishLegalDocumentUpdateExisting(t *testing.T) {
	newTestDB(t)
	doc := &LegalDocument{
		Type: "user_agreement", Version: "v1.0", Language: "en",
		Content: "Original",
	}
	PublishLegalDocument(doc)

	// Update same version.
	doc.Content = "Updated"
	if err := PublishLegalDocument(doc); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	got, _ := GetLegalDocument("user_agreement", "en", "")
	if got.Content != "Updated" {
		t.Fatalf("expected updated content, got %s", got.Content)
	}
}

func TestGetLatestLegalVersion(t *testing.T) {
	newTestDB(t)
	PublishLegalDocument(&LegalDocument{Type: "user_agreement", Version: "v1.0", Language: "en", Content: "v1"})
	PublishLegalDocument(&LegalDocument{Type: "user_agreement", Version: "v2.0", Language: "en", Content: "v2"})

	version, err := GetLatestLegalVersion("user_agreement")
	if err != nil {
		t.Fatal(err)
	}
	if version != "v2.0" {
		t.Fatalf("expected v2.0, got %s", version)
	}
}

func TestGetLatestLegalVersionNone(t *testing.T) {
	newTestDB(t)
	version, err := GetLatestLegalVersion("user_agreement")
	if err != nil {
		t.Fatal(err)
	}
	if version != "" {
		t.Fatalf("expected empty version when none published, got %s", version)
	}
}

func TestGetLegalDocumentNotFound(t *testing.T) {
	newTestDB(t)
	_, err := GetLegalDocument("user_agreement", "en", "")
	if err != ErrLegalDocumentNotFound {
		t.Fatalf("expected ErrLegalDocumentNotFound, got %v", err)
	}
}

func TestPublishLegalDocumentValidation(t *testing.T) {
	newTestDB(t)
	cases := []struct {
		name string
		doc  *LegalDocument
	}{
		{"empty type", &LegalDocument{Version: "v1", Content: "x"}},
		{"empty version", &LegalDocument{Type: "user_agreement", Content: "x"}},
		{"empty content", &LegalDocument{Type: "user_agreement", Version: "v1"}},
	}
	for _, tc := range cases {
		if err := PublishLegalDocument(tc.doc); err == nil {
			t.Errorf("%s: expected error, got nil", tc.name)
		}
	}
}

func TestListLegalDocuments(t *testing.T) {
	newTestDB(t)
	PublishLegalDocument(&LegalDocument{Type: "privacy_policy", Version: "v1", Language: "en", Content: "p1"})
	PublishLegalDocument(&LegalDocument{Type: "user_agreement", Version: "v1", Language: "en", Content: "u1"})

	all, err := ListLegalDocuments("")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 docs, got %d", len(all))
	}

	filtered, _ := ListLegalDocuments("user_agreement")
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered doc, got %d", len(filtered))
	}
}

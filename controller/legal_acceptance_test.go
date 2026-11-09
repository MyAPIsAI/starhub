package controller

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"starhub/model"
)

func TestUIAC23RegionalLegalBoundariesAndPrivacy(t *testing.T) {
	newTestDB(t)
	profile := model.DefaultDeploymentProfile()
	profile.Timezone = "Asia/Dubai"
	profile.CookieConsent = true
	profile.CookieConsentMode = "dialog"
	profile.PaymentConfigRefs = map[string]string{"card": "config:private-payment"}
	if _, err := model.SaveDeploymentProfile(t.Context(), profile, 0, 1, ""); err != nil {
		t.Fatal(err)
	}
	boundary := time.Date(2026, 12, 31, 20, 0, 0, 0, time.UTC)
	for _, kind := range []string{"terms", "privacy", "cookies", "refund"} {
		for _, lang := range []string{"en", "zh", "ar"} {
			for i, local := range []string{"2026-11-01T00:00", "2026-12-01T00:00", "2027-01-01T00:00"} {
				_, err := model.SaveLegalDocument(t.Context(), model.LegalDocumentVersion{Kind: kind, Language: lang, Version: fmt.Sprintf("v%d", i+1), Content: fmt.Sprintf("Synthetic %s %s v%d", kind, lang, i+1)}, local, 1, "")
				if err != nil {
					t.Fatal(err)
				}
			}
			for _, check := range []struct {
				at      time.Time
				version string
			}{{time.Date(2026, 12, 10, 0, 0, 0, 0, time.UTC), "v2"}, {boundary.Add(-time.Second), "v2"}, {boundary, "v3"}} {
				r := httptest.NewRequest("GET", "/api/legal/"+kind+"?language="+lang, nil)
				r.SetPathValue("kind", kind)
				w := httptest.NewRecorder()
				PublishedLegalAt(check.at)(w, r)
				var body struct {
					Data struct {
						Version     string
						EffectiveAt int64 `json:"effective_at"`
						Next        int64 `json:"next_effective_at"`
						Content     string
						Timezone    string
					}
				}
				if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if w.Code != 200 || body.Data.Version != check.version || body.Data.Timezone != "Asia/Dubai" || w.Header().Get("Cache-Control") != "no-store" {
					t.Fatal(w.Body.String())
				}
				if check.version == "v2" && (body.Data.Next != boundary.Unix() || strings.Contains(body.Data.Content, "v3")) {
					t.Fatal("future content leaked or schedule missing")
				}
				if check.version == "v3" && (body.Data.EffectiveAt != boundary.Unix() || body.Data.Next != 0) {
					t.Fatal("regional boundary mismatch")
				}
				if strings.Contains(w.Body.String(), "actor_id") || strings.Contains(w.Body.String(), "created_at") {
					t.Fatal("internal metadata exposed")
				}
			}
		}
	}
	for _, kind := range []string{"terms", "unknown"} {
		r := httptest.NewRequest("GET", "/api/legal/"+kind+"?language=fr", nil)
		r.SetPathValue("kind", kind)
		w := httptest.NewRecorder()
		PublishedLegalAt(boundary)(w, r)
		if w.Code != 404 {
			t.Fatal("missing translation was fabricated")
		}
	}
	w := httptest.NewRecorder()
	GetPublicLegalPolicy(w, httptest.NewRequest("GET", "/api/legal-policy", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"cookie_consent_mode":"dialog"`) || strings.Contains(w.Body.String(), "private-payment") {
		t.Fatal(w.Body.String())
	}
	profile.CookieConsentMode = "invalid"
	if model.ValidateDeploymentProfile(profile) == nil {
		t.Fatal("invalid consent mode accepted")
	}
}

func TestLegalPaginationStatusUsesEntireHistory(t *testing.T) {
	newTestDB(t)
	for i := 0; i < 27; i++ {
		if err := model.DB.Create(&model.LegalDocumentVersion{Kind: "terms", Language: "en", Version: fmt.Sprintf("v%d", i), Content: "Synthetic", EffectiveAt: 1000 + int64(i), Timezone: "UTC"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := model.DB.Create(&model.LegalDocumentVersion{Kind: "terms", Language: "en", Version: "future", Content: "Unpublished", EffectiveAt: time.Now().Add(24 * time.Hour).Unix(), Timezone: "UTC"}).Error; err != nil {
		t.Fatal(err)
	}
	for page := 1; page <= 2; page++ {
		w := httptest.NewRecorder()
		GetLegalVersions(w, httptest.NewRequest("GET", fmt.Sprintf("/api/admin/legal?page=%d&page_size=20&kind=terms&language=en", page), nil))
		var body struct {
			Data struct {
				Total int
				Items []struct {
					Status  string
					Version string
				}
			}
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if w.Code != 200 || body.Data.Total != 28 {
			t.Fatal(w.Body.String())
		}
		if page == 1 && (len(body.Data.Items) != 20 || body.Data.Items[0].Status != "scheduled" || body.Data.Items[1].Status != "current") {
			t.Fatal(w.Body.String())
		}
		if page == 2 {
			if len(body.Data.Items) != 8 {
				t.Fatal(w.Body.String())
			}
			for _, row := range body.Data.Items {
				if row.Status != "historical" {
					t.Fatal("page two incorrectly reports a current version")
				}
			}
		}
	}
}

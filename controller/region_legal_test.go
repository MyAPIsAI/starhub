package controller

import (
	"starhub/model"
	"testing"
)

func TestLegalPageUsesActiveRegionVersionAndLanguage(t *testing.T) {
	newTestDB(t)
	t.Setenv("STARHUB_REGION_PROFILE", "legal")
	if err := model.SaveRegionProfile(&model.RegionProfile{ID: "legal", Name: "Legal", DefaultLanguage: "ar", SupportedLanguages: "ar,en", LegalVersion: "v1", Enabled: true}, -1); err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"v1", "v2"} {
		if err := model.PublishLegalDocument(&model.LegalDocument{Type: "user_agreement", Version: version, Language: "ar", RegionID: "legal", Content: "Active regional terms " + version}); err != nil {
			t.Fatal(err)
		}
	}
	response := call(GetLegalDocumentHandler, testRequest{target: "/api/legal/user_agreement", path: map[string]string{"type": "user_agreement"}, headers: map[string]string{"Accept-Language": "fr"}})
	var document model.LegalDocument
	decodeData(t, response, &document)
	if document.RegionID != "legal" || document.Version != "v1" || document.Language != "ar" {
		t.Fatalf("wrong active legal document: %+v", document)
	}
}

package controller

import (
	"starhub/common"
	"starhub/model"
	"testing"
)

func TestAccountLanguageWritesRespectActiveSupportedLanguages(t *testing.T) {
	newTestDB(t)
	t.Setenv("STARHUB_REGION_PROFILE", "languages")
	if err := model.SaveRegionProfile(&model.RegionProfile{ID: "languages", Name: "Languages", DefaultLanguage: "ar", SupportedLanguages: "ar,en", Enabled: true}, -1); err != nil {
		t.Fatal(err)
	}
	user := newUser(t, "language-owner", common.RoleUser, 0)
	for _, tc := range []struct {
		language string
		status   int
	}{{"ko", 400}, {"ar", 200}} {
		response := call(UpdateSelf, testRequest{method: "PUT", target: "/api/user/self", userID: user.ID, role: user.Role, body: `{"language":"` + tc.language + `"}`})
		expectStatus(t, response, tc.status)
	}
	fresh, err := model.GetUserByID(user.ID)
	if err != nil || fresh.Language != "ar" {
		t.Fatal("supported language preference was not saved")
	}
}

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

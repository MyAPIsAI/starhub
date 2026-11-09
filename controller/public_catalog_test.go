package controller

import (
	"encoding/json"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"starhub/model"
	"starhub/setting"
)

func catalogFixture(t *testing.T) {
	t.Helper()
	newTestDB(t)
	if _, err := model.SaveOption(setting.OptionKeyGroupRatio, `{"standard":0.6,"lite":0.4,"private":0.01}`, 0); err != nil {
		t.Fatal(err)
	}
	for _, g := range []model.ServiceGroupConfig{{GroupID: "standard", Name: "Standard", Enabled: true, Visibility: "common"}, {GroupID: "lite", Name: "Lite", Enabled: true, Visibility: "common"}, {GroupID: "private", Name: "Private", Enabled: true, Visibility: "custom", AccountIDs: "1"}} {
		if err := model.UpsertServiceGroup(&g); err != nil {
			t.Fatal(err)
		}
	}
	if err := setting.UpdateRatio(setting.OptionKeyModelRatio, `{"public-chat":2,"public-audio":3}`); err != nil {
		t.Fatal(err)
	}
	if err := setting.UpdateRatio(setting.OptionKeyCompletionRatio, `{"public-chat":4}`); err != nil {
		t.Fatal(err)
	}
	if err := model.InsertChannel(&model.Channel{Name: "Public catalog fixture", Type: 1, Status: 1, Group: "standard,private", Models: "public-chat,public-audio,unlisted"}); err != nil {
		t.Fatal(err)
	}
	for _, m := range []model.StandardModel{{Model: "public-chat", Name: "Public chat", Vendor: "openai", Kind: "chat", Listed: true, Protocols: []string{"OpenAI", "Anthropic"}, ContextLength: 32000}, {Model: "public-audio", Name: "Audio fixture", Vendor: "audio-vendor", Kind: "audio", Listed: true, Protocols: []string{"OpenAI"}}, {Model: "unlisted", Vendor: "openai", Kind: "chat", Listed: false}} {
		if err := model.DB.Create(&m).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := model.UpsertModelMeta(&model.ModelMeta{Model: "public-chat", Tags: "text", OfficialInputPerM: f64(2), OfficialOutputPerM: f64(8)}); err != nil {
		t.Fatal(err)
	}
}

func TestUIAC01And03PublicCatalogPriceAndGroupTable(t *testing.T) {
	catalogFixture(t)
	r := httptest.NewRequest("GET", "/api/catalog/models", nil)
	w := httptest.NewRecorder()
	PublicCatalogList(w, r)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var response struct {
		Data struct {
			Items []plazaRow `json:"items"`
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Data.Items) != 3 || response.Data.Items[0].ID != "myapis-auto" {
		t.Fatal(w.Body.String())
	}
	for _, row := range response.Data.Items {
		if row.ID != "public-chat" {
			continue
		}
		if row.PriceGroup != "standard" || *row.Pricing.InputPerM != 1.2 || *row.Pricing.OutputPerM != 4.8 {
			t.Fatal(row)
		}
		if len(row.GroupPrices) != 2 || row.GroupPrices[0].ID != "lite" || row.GroupPrices[0].Available || !row.GroupPrices[1].Available {
			t.Fatal(row.GroupPrices)
		}
		if slices.Contains(row.AvailableGroups, "private") || strings.Contains(w.Body.String(), "Private") {
			t.Fatal("custom group leaked")
		}
	}
	r = httptest.NewRequest("GET", "/api/catalog/models/public-chat", nil)
	r.SetPathValue("id", "public-chat")
	w = httptest.NewRecorder()
	PublicCatalogDetail(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"group_prices"`) {
		t.Fatal(w.Body.String())
	}
}

func TestUIAC02PublicCatalogCombinedFilters(t *testing.T) {
	catalogFixture(t)
	for _, query := range []string{"vendors=audio-vendor&tags=audio", "vendors=openai&tags=text&q=public-chat"} {
		w := httptest.NewRecorder()
		PublicCatalogList(w, httptest.NewRequest("GET", "/api/catalog/models?"+query, nil))
		var result struct {
			Data struct {
				Total int `json:"total"`
			}
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.Data.Total != 1 {
			t.Fatalf("%s: %s", query, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	PublicCatalogList(w, httptest.NewRequest("GET", "/api/catalog/models?vendors=audio-vendor&tags=text", nil))
	if !strings.Contains(w.Body.String(), `"total":0`) {
		t.Fatal(w.Body.String())
	}
}

func TestUIAC18PublicCatalogHiddenDetailAndImmediateRecovery(t *testing.T) {
	catalogFixture(t)
	if _, err := publicCatalogRows(t.Context(), time.Now().Add(-20*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := model.DB.Model(&model.Ability{}).Where("model = ?", "public-chat").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := publicCatalogRows(t.Context(), time.Now().Add(-12*time.Minute)); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	PublicCatalogList(w, httptest.NewRequest("GET", "/api/catalog/models?q=public-chat", nil))
	if !strings.Contains(w.Body.String(), `"total":0`) {
		t.Fatal(w.Body.String())
	}
	r := httptest.NewRequest("GET", "/api/catalog/models/public-chat", nil)
	r.SetPathValue("id", "public-chat")
	w = httptest.NewRecorder()
	PublicCatalogDetail(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"unavailable_reason":"no_public_resource"`) {
		t.Fatal(w.Body.String())
	}
	if err := model.DB.Model(&model.Ability{}).Where("model = ?", "public-chat").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	PublicCatalogList(w, httptest.NewRequest("GET", "/api/catalog/models?q=public-chat", nil))
	if !strings.Contains(w.Body.String(), `"total":1`) {
		t.Fatal(w.Body.String())
	}
	if err := model.DB.Model(&model.StandardModel{}).Where("model = ?", "public-chat").Update("listed", false).Error; err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	PublicCatalogDetail(w, r)
	if w.Code != 404 {
		t.Fatal(w.Code, w.Body.String())
	}
	if err := model.DB.Model(&model.Ability{}).Where("model = ?", "public-chat").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := model.DB.Model(&model.StandardModel{}).Where("model = ?", "public-chat").Update("listed", true).Error; err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	PublicCatalogList(w, httptest.NewRequest("GET", "/api/catalog/models?q=public-chat", nil))
	if !strings.Contains(w.Body.String(), `"total":0`) {
		t.Fatal("relisting must not inherit previous availability", w.Body.String())
	}
}

func TestPublicCatalogConditionalPriceHasNoAdvertisedSingleAmount(t *testing.T) {
	catalogFixture(t)
	if err := setting.UpdateOption(setting.OptionKeyModelPriceExpr, `{"public-chat":"input * 0.000002"}`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = setting.UpdateOption(setting.OptionKeyModelPriceExpr, `{}`) })
	rows, err := publicCatalogRows(t.Context(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.ID != "public-chat" {
			continue
		}
		p := row.Pricing
		if p.Mode != "tiered" || p.InputPerM != nil || p.OutputPerM != nil || p.PerRequest != nil || p.Expression != "(input * 0.000002) * 0.6" {
			t.Fatal(p)
		}
		return
	}
	t.Fatal("missing listed model")
}

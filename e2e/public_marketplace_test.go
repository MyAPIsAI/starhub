package main

import (
	"encoding/json"
	"reflect"
	"testing"

	"starhub/model"
	"starhub/setting"
)

func TestUIAC01AnonymousCatalogRoutesIgnorePrivatePrices(t *testing.T) {
	env := setupRelayApp(t)
	if _, err := model.SaveOption(setting.OptionKeyGroupRatio, `{"free":0,"public":0.6,"private":0.01}`, env.rootID); err != nil {
		t.Fatal(err)
	}
	for _, group := range []model.ServiceGroupConfig{{GroupID: "free", Enabled: true, Visibility: "common"}, {GroupID: "public", Enabled: true, Visibility: "common"}, {GroupID: "private", Enabled: true, Visibility: "custom", AccountIDs: "1"}} {
		if err := model.UpsertServiceGroup(&group); err != nil {
			t.Fatal(err)
		}
	}
	if err := model.InsertChannel(&model.Channel{Name: "Public fixture", Type: 1, Models: "marketplace-chat", Group: "free,public,private", Status: 1}); err != nil {
		t.Fatal(err)
	}
	if err := model.InsertChannel(&model.Channel{Name: "Private fixture", Type: 1, Models: "private-only", Group: "private", Status: 1}); err != nil {
		t.Fatal(err)
	}
	privateResponse := doRequest(env.base, "GET", "/api/catalog/models/private-only", "", nil)
	if privateResponse.Status != 404 {
		t.Fatal("private-only model was published", privateResponse.Status)
	}
	if err := model.DB.Create(&model.StandardModel{Model: "marketplace-chat", Name: "Marketplace chat", Kind: "chat", Vendor: "fixture", Listed: true}).Error; err != nil {
		t.Fatal(err)
	}
	var publicBody map[string]any
	for _, session := range []string{"", env.session} {
		r := doRequest(env.base, "GET", "/api/catalog/models?vendors=fixture&tags=text&group=private", session, nil)
		var body map[string]any
		if r.Status != 200 || json.Unmarshal([]byte(r.Body), &body) != nil {
			t.Fatal(r.Status, r.Body)
		}
		if publicBody == nil {
			publicBody = body
		} else if !reflect.DeepEqual(publicBody, body) {
			t.Fatal("public quote depends on session")
		}
		data := body["data"].(map[string]any)
		items := data["items"].([]any)
		if len(items) != 1 {
			t.Fatal(r.Body)
		}
		row := items[0].(map[string]any)
		if row["price_group"] != "free" || row["pricing"].(map[string]any)["input_per_m"] != float64(0) || len(row["group_prices"].([]any)) != 2 {
			t.Fatal(r.Body)
		}
	}
	for _, path := range []string{"/api/catalog/models/marketplace-chat", "/api/catalog/models/myapis-auto", "/api/catalog/models/veyra-auto", "/api/catalog/filters"} {
		r := doRequest(env.base, "GET", path, "", nil)
		if r.Status != 200 {
			t.Fatal(path, r.Status, r.Body)
		}
	}
	if err := model.DB.Model(&model.StandardModel{}).Where("model = ?", "marketplace-chat").Update("listed", false).Error; err != nil {
		t.Fatal(err)
	}
	r := doRequest(env.base, "GET", "/api/catalog/models/marketplace-chat", "", nil)
	if r.Status != 404 {
		t.Fatal(r.Status, r.Body)
	}
}

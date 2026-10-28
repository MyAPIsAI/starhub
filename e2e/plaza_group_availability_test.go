package main

import (
	"encoding/json"
	"reflect"
	"testing"

	"starhub/common"
	"starhub/model"
	"starhub/setting"
)

func TestPublicCatalogDoesNotAdvertiseAGroupDisabledByManagement(t *testing.T) {
	env := setupRelayApp(t)
	if err := model.InsertChannel(&model.Channel{Name: "Availability fixture", Type: 1, Models: "gpt-4o", Group: "default,vip", Status: common.ChannelStatusEnabled}); err != nil {
		t.Fatal(err)
	}
	if _, err := model.SaveOption(setting.OptionKeyGroupRatio, `{"default":1,"vip":0}`, env.rootID); err != nil {
		t.Fatal(err)
	}
	setEnabled := func(enabled bool) {
		t.Helper()
		response := doRequest(env.base, "PUT", "/api/service-group", env.session, map[string]any{"group_id": "default", "name": "Default", "multiplier": "1", "enabled": enabled})
		if response.Status != 200 {
			t.Fatalf("group state: %d %s", response.Status, response.Body)
		}
	}
	setEnabled(false)
	for _, session := range []string{"", env.session} {
		response := doRequest(env.base, "GET", "/api/plaza/models/gpt-4o?group=vip", session, nil)
		var detail struct {
			Data struct {
				Group string `json:"group"`
				Model struct {
					Available bool     `json:"is_available"`
					Groups    []string `json:"available_groups"`
					Protocols []string `json:"protocols"`
					Reason    string   `json:"unavailable_reason"`
				} `json:"model"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(response.Body), &detail); err != nil {
			t.Fatal(err)
		}
		if response.Status != 200 || detail.Data.Group != "default" || detail.Data.Model.Available || detail.Data.Model.Reason != "group_not_configured" || len(detail.Data.Model.Protocols) != 0 || !reflect.DeepEqual(detail.Data.Model.Groups, []string{"vip"}) {
			t.Fatalf("disabled group advertised: %d %s", response.Status, response.Body)
		}
		response = doRequest(env.base, "GET", "/api/plaza/models?available_only=1", session, nil)
		var list struct {
			Data struct {
				Total int   `json:"total"`
				Items []any `json:"items"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(response.Body), &list); err != nil {
			t.Fatal(err)
		}
		if response.Status != 200 || list.Data.Total != 0 || len(list.Data.Items) != 0 {
			t.Fatalf("available-only ignored group state: %d %s", response.Status, response.Body)
		}
	}
	setEnabled(true)
	response := doRequest(env.base, "GET", "/api/plaza/models?available_only=1", "", nil)
	var list struct {
		Data struct {
			Total int `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(response.Body), &list); err != nil {
		t.Fatal(err)
	}
	if response.Status != 200 || list.Data.Total != 1 {
		t.Fatalf("reenabled model not restored: %d %s", response.Status, response.Body)
	}
}

package controller

import (
	"net/http"
	"testing"

	"starhub/common"
	"starhub/setting"
)

func TestUpsertServiceGroupCreate(t *testing.T) {
	newTestDB(t)
	admin := newUser(t, "admin", common.RoleAdmin, 1000)

	rr := call(UpsertServiceGroup, testRequest{
		method: http.MethodPut,
		target: "/api/service-group",
		body:   `{"group_id":"vip","name":"VIP","multiplier":"0.5","visibility":"common","enabled":true}`,
		userID: admin.ID,
		role:   common.RoleAdmin,
	})
	var got map[string]any
	decodeData(t, rr, &got)
	if got["group_id"] != "vip" {
		t.Fatalf("group_id = %v, want vip", got["group_id"])
	}
	if got["name"] != "VIP" {
		t.Fatalf("name = %v, want VIP", got["name"])
	}
	if got["multiplier"] != "0.5" {
		t.Fatalf("multiplier = %v, want 0.5", got["multiplier"])
	}

	// Verify group_ratio was synced.
	if !setting.ContainsGroupRatio("vip") {
		t.Fatal("group_ratio should contain 'vip' after upsert")
	}
	v, ok := setting.GroupRatioValue("vip")
	if !ok || v != 0.5 {
		t.Fatalf("group_ratio['vip'] = %v, want 0.5", v)
	}
}

func TestUpsertServiceGroupUpdate(t *testing.T) {
	newTestDB(t)
	admin := newUser(t, "admin", common.RoleAdmin, 1000)

	call(UpsertServiceGroup, testRequest{
		method: http.MethodPut,
		target: "/api/service-group",
		body:   `{"group_id":"pro","name":"Pro","multiplier":"0.8","visibility":"common","enabled":true}`,
		userID: admin.ID,
		role:   common.RoleAdmin,
	})
	rr := call(UpsertServiceGroup, testRequest{
		method: http.MethodPut,
		target: "/api/service-group",
		body:   `{"group_id":"pro","name":"Pro Plus","multiplier":"0.6","visibility":"common","enabled":true}`,
		userID: admin.ID,
		role:   common.RoleAdmin,
	})
	var got map[string]any
	decodeData(t, rr, &got)
	if got["name"] != "Pro Plus" {
		t.Fatalf("name = %v, want Pro Plus", got["name"])
	}
	v, _ := setting.GroupRatioValue("pro")
	if v != 0.6 {
		t.Fatalf("group_ratio['pro'] = %v, want 0.6", v)
	}
}

func TestUpsertServiceGroupValidation(t *testing.T) {
	newTestDB(t)
	admin := newUser(t, "admin", common.RoleAdmin, 1000)

	cases := []struct {
		name string
		body string
	}{
		{"empty group_id", `{"group_id":"","name":"X","multiplier":"1"}`},
		{"invalid group_id", `{"group_id":"UPPER","name":"X","multiplier":"1"}`},
		{"empty name", `{"group_id":"x","name":"","multiplier":"1"}`},
		{"zero multiplier", `{"group_id":"x","name":"X","multiplier":"0"}`},
		{"negative multiplier", `{"group_id":"x","name":"X","multiplier":"-1"}`},
		{"too large multiplier", `{"group_id":"x","name":"X","multiplier":"101"}`},
		{"custom without accounts", `{"group_id":"x","name":"X","multiplier":"1","visibility":"custom"}`},
		{"invalid visibility", `{"group_id":"x","name":"X","multiplier":"1","visibility":"other"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := call(UpsertServiceGroup, testRequest{
				method: http.MethodPut,
				target: "/api/service-group",
				body:   tc.body,
				userID: admin.ID,
				role:   common.RoleAdmin,
			})
			expectError(t, rr, http.StatusBadRequest, "")
		})
	}
}

func TestDeleteServiceGroup(t *testing.T) {
	newTestDB(t)
	admin := newUser(t, "admin", common.RoleAdmin, 1000)

	call(UpsertServiceGroup, testRequest{
		method: http.MethodPut,
		target: "/api/service-group",
		body:   `{"group_id":"temp","name":"Temp","multiplier":"1","visibility":"common","enabled":true}`,
		userID: admin.ID,
		role:   common.RoleAdmin,
	})
	if !setting.ContainsGroupRatio("temp") {
		t.Fatal("group_ratio should contain 'temp' before delete")
	}
	rr := call(DeleteServiceGroup, testRequest{
		method: http.MethodDelete,
		target: "/api/service-group/temp",
		userID: admin.ID,
		role:   common.RoleAdmin,
		path:   map[string]string{"id": "temp"},
	})
	expectStatus(t, rr, http.StatusOK)
	if setting.ContainsGroupRatio("temp") {
		t.Fatal("group_ratio should NOT contain 'temp' after delete")
	}
}

func TestDeleteServiceGroupNotFound(t *testing.T) {
	newTestDB(t)
	admin := newUser(t, "admin", common.RoleAdmin, 1000)

	rr := call(DeleteServiceGroup, testRequest{
		method: http.MethodDelete,
		target: "/api/service-group/nonexistent",
		userID: admin.ID,
		role:   common.RoleAdmin,
		path:   map[string]string{"id": "nonexistent"},
	})
	expectError(t, rr, http.StatusNotFound, "not found")
}

func TestListServiceGroups(t *testing.T) {
	newTestDB(t)
	admin := newUser(t, "admin", common.RoleAdmin, 1000)

	call(UpsertServiceGroup, testRequest{
		method: http.MethodPut,
		target: "/api/service-group",
		body:   `{"group_id":"alpha","name":"Alpha","multiplier":"1","visibility":"common","enabled":true}`,
		userID: admin.ID,
		role:   common.RoleAdmin,
	})
	call(UpsertServiceGroup, testRequest{
		method: http.MethodPut,
		target: "/api/service-group",
		body:   `{"group_id":"beta","name":"Beta","multiplier":"0.5","visibility":"common","enabled":true}`,
		userID: admin.ID,
		role:   common.RoleAdmin,
	})
	rr := call(ListServiceGroups, testRequest{
		method: http.MethodGet,
		target: "/api/service-group",
		userID: admin.ID,
		role:   common.RoleAdmin,
	})
	var groups []map[string]any
	decodeData(t, rr, &groups)
	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2", len(groups))
	}
}

func TestUpsertServiceGroupDisabledNotInRatio(t *testing.T) {
	newTestDB(t)
	admin := newUser(t, "admin", common.RoleAdmin, 1000)

	call(UpsertServiceGroup, testRequest{
		method: http.MethodPut,
		target: "/api/service-group",
		body:   `{"group_id":"disabled","name":"Disabled","multiplier":"0.3","visibility":"common","enabled":false}`,
		userID: admin.ID,
		role:   common.RoleAdmin,
	})
	// Disabled groups should not appear in group_ratio so tokens
	// cannot be created against them.
	if setting.ContainsGroupRatio("disabled") {
		t.Fatal("group_ratio should NOT contain disabled groups")
	}
}

func TestUpsertServiceGroupCustomVisibility(t *testing.T) {
	newTestDB(t)
	admin := newUser(t, "admin", common.RoleAdmin, 1000)

	rr := call(UpsertServiceGroup, testRequest{
		method: http.MethodPut,
		target: "/api/service-group",
		body:   `{"group_id":"enterprise","name":"Enterprise","multiplier":"0.4","visibility":"custom","account_ids":["acct-1","acct-2"],"enabled":true}`,
		userID: admin.ID,
		role:   common.RoleAdmin,
	})
	var got map[string]any
	decodeData(t, rr, &got)
	if got["visibility"] != "custom" {
		t.Fatalf("visibility = %v, want custom", got["visibility"])
	}
	ids := got["account_ids"].([]any)
	if len(ids) != 2 {
		t.Fatalf("account_ids length = %d, want 2", len(ids))
	}
}

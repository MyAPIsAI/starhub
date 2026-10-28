package main

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
)

func TestGroupCatalogPolicyAndIndependentPriceRevisionGuard(t *testing.T) {
	env := setupRelayApp(t)
	type groupRow struct {
		ID         int64  `json:"id"`
		Revision   int64  `json:"revision"`
		Multiplier string `json:"multiplier"`
	}
	type configuration struct {
		Groups      []groupRow         `json:"groups"`
		Version     int64              `json:"option_version"`
		Loaded      int64              `json:"loaded_option_version"`
		Ratios      map[string]float64 `json:"group_ratios"`
		Guards      bool               `json:"revision_guards"`
		OptionGuard bool               `json:"option_version_guard"`
	}
	read := func() configuration {
		t.Helper()
		response := doRequest(env.base, "GET", "/api/service-group/configuration", env.session, nil)
		if response.Status != 200 {
			t.Fatalf("catalog: %d %s", response.Status, response.Body)
		}
		var reply struct {
			Data configuration `json:"data"`
		}
		if err := json.Unmarshal([]byte(response.Body), &reply); err != nil {
			t.Fatal(err)
		}
		return reply.Data
	}
	price := func(value string) {
		t.Helper()
		response := doRequest(env.base, "PUT", "/api/option", env.session, map[string]any{"key": "group_ratio", "value": value})
		if response.Status != 200 {
			t.Fatalf("price update: %d %s", response.Status, response.Body)
		}
	}
	for _, role := range []int{common.RoleUser, common.RoleFinance, common.RoleAdmin, 99, 101} {
		user := &model.User{Username: fmt.Sprintf("catalog-role-%d", role), Role: role, Status: common.StatusEnabled, AffCode: common.GenerateAffCode()}
		if err := model.CreateUser(user); err != nil {
			t.Fatal(err)
		}
		session := middleware.Sessions.Create(user.ID, time.Hour)
		t.Cleanup(func() { middleware.Sessions.Remove(session) })
		want := 403
		if role == common.RoleAdmin {
			want = 200
		}
		if response := doRequest(env.base, "GET", "/api/service-group/configuration", session, nil); response.Status != want {
			t.Fatalf("role %d catalog: %d", role, response.Status)
		}
	}
	if response := doRequest(env.base, "GET", "/api/service-group/configuration", "", nil); response.Status != 401 {
		t.Fatalf("anonymous catalog: %d", response.Status)
	}
	price(`{"default":1,"legacy-only":0}`)
	initial := read()
	if !initial.Guards || !initial.OptionGuard || len(initial.Groups) != 0 || initial.Version != initial.Loaded {
		t.Fatalf("initial catalog: %+v", initial)
	}
	payload := map[string]any{"group_id": "managed", "name": "Managed", "multiplier": "0.8", "expected_group_id": 0, "expected_revision": 0, "expected_option_version": initial.Version}
	if response := doRequest(env.base, "PUT", "/api/service-group", env.session, payload); response.Status != 200 {
		t.Fatalf("guarded create: %d %s", response.Status, response.Body)
	}
	before := read()
	if len(before.Groups) != 1 || before.Ratios["managed"] != 0.8 {
		t.Fatal("created catalog not coherent")
	}
	price(`{"default":1,"managed":0.5,"legacy-only":0}`)
	payload["expected_group_id"] = before.Groups[0].ID
	payload["expected_revision"] = before.Groups[0].Revision
	payload["expected_option_version"] = before.Version
	payload["name"] = "Approved"
	payload["multiplier"] = "0.6"
	if response := doRequest(env.base, "PUT", "/api/service-group", env.session, payload); response.Status != 409 {
		t.Fatalf("independent price not guarded: %d %s", response.Status, response.Body)
	}
	current := read()
	if current.Groups[0].Revision != before.Groups[0].Revision || current.Groups[0].Multiplier != "0.8" || current.Ratios["managed"] != 0.5 {
		t.Fatal("stale edit changed metadata or hid independently configured price")
	}
	payload["expected_option_version"] = current.Version
	if response := doRequest(env.base, "PUT", "/api/service-group", env.session, payload); response.Status != 200 {
		t.Fatalf("current combined save: %d %s", response.Status, response.Body)
	}
	updated := read()
	remove := map[string]any{"expected_group_id": updated.Groups[0].ID, "expected_revision": updated.Groups[0].Revision, "expected_option_version": current.Version}
	if response := doRequest(env.base, "DELETE", "/api/service-group/managed", env.session, remove); response.Status != 409 {
		t.Fatalf("stale option delete: %d %s", response.Status, response.Body)
	}
	remove["expected_option_version"] = updated.Version
	if response := doRequest(env.base, "DELETE", "/api/service-group/managed", env.session, remove); response.Status != 200 {
		t.Fatalf("current delete: %d %s", response.Status, response.Body)
	}
	final := read()
	if len(final.Groups) != 0 || len(final.Ratios) != 2 || final.Version != final.Loaded {
		t.Fatalf("final catalog: %+v", final)
	}
	if value, present := final.Ratios["legacy-only"]; !present || value != 0 {
		t.Fatal("legacy configured zero not retained")
	}
}

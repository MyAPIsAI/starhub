package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
	"starhub/relay"
	"starhub/setting"
)

func TestTimePricingAPIUsesSharedVersionAndExplicitActiveCapability(t *testing.T) {
	env := setupRelayApp(t)
	read := func() struct {
		Data struct {
			Version int64           `json:"version"`
			Loaded  int64           `json:"loaded_version"`
			Rules   json.RawMessage `json:"rules"`
			Guards  bool            `json:"revision_guards"`
			Schema  string          `json:"schema"`
			Active  bool            `json:"billing_active"`
		} `json:"data"`
	} {
		t.Helper()
		response := doRequest(env.base, "GET", "/api/option/time-pricing", env.session, nil)
		var result struct {
			Data struct {
				Version int64           `json:"version"`
				Loaded  int64           `json:"loaded_version"`
				Rules   json.RawMessage `json:"rules"`
				Guards  bool            `json:"revision_guards"`
				Schema  string          `json:"schema"`
				Active  bool            `json:"billing_active"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(response.Body), &result); err != nil || response.Status != 200 {
			t.Fatalf("read time pricing: %d %s %v", response.Status, response.Body, err)
		}
		return result
	}
	initial := read()
	if initial.Data.Version != 0 || initial.Data.Loaded != 0 || string(initial.Data.Rules) != "{}" || !initial.Data.Guards || initial.Data.Schema != "weekly_multiplier_v1" || !initial.Data.Active {
		t.Fatal("initial time-pricing capability is inaccurate")
	}
	for _, role := range []int{common.RoleUser, common.RoleFinance, 99, 101, common.RoleAdmin} {
		user := &model.User{Username: fmt.Sprintf("time-role-%d", role), Role: role, Status: common.StatusEnabled, AffCode: common.GenerateAffCode()}
		if err := model.CreateUser(user); err != nil {
			t.Fatal(err)
		}
		session := middleware.Sessions.Create(user.ID, time.Hour)
		t.Cleanup(func() { middleware.Sessions.Remove(session) })
		want := 403
		if role == common.RoleAdmin {
			want = 200
		}
		for _, method := range []string{"GET", "PUT"} {
			response := doRequest(env.base, method, "/api/option/time-pricing", session, map[string]any{"expected_version": 0, "rules": map[string]any{}})
			if response.Status != want {
				t.Fatalf("role %d %s: %d %s", role, method, response.Status, response.Body)
			}
		}
	}
	for _, method := range []string{"GET", "PUT"} {
		if response := doRequest(env.base, method, "/api/option/time-pricing", "", map[string]any{"expected_version": 0, "rules": map[string]any{}}); response.Status != 401 {
			t.Fatal("anonymous time configuration allowed")
		}
	}
	rules := func(label string) json.RawMessage {
		return json.RawMessage(fmt.Sprintf(`{"gpt-4o":{"timezone":"UTC","windows":[{"id":"all","label":%q,"days":[0,1,2,3,4,5,6],"start":"00:00","end":"24:00","multiplier":2}]}}`, label))
	}
	for _, payload := range []map[string]any{{"rules": map[string]any{}}, {"expected_version": nil, "rules": map[string]any{}}, {"expected_version": 0.5, "rules": map[string]any{}}, {"expected_version": -1, "rules": map[string]any{}}, {"expected_version": 0, "rules": nil}, {"expected_version": 0, "rules": json.RawMessage(`{"m":{"timezone":"Local","windows":[]}}`)}, {"expected_version": 0, "rules": map[string]any{}, "unknown": true}} {
		if response := doRequest(env.base, "PUT", "/api/option/time-pricing", env.session, payload); response.Status != 400 {
			t.Fatalf("invalid time payload: %d %s", response.Status, response.Body)
		}
	}
	usage := &relay.Usage{PromptTokens: 100, CompletionTokens: 50}
	before := relay.ComputeUsageQuota(relay.NewPriceData("gpt-4o", "default"), usage)
	if response := doRequest(env.base, "PUT", "/api/option/time-pricing", env.session, map[string]any{"expected_version": 0, "rules": rules("v1")}); response.Status != 200 {
		t.Fatalf("save time rules: %d %s", response.Status, response.Body)
	}
	saved := read()
	if saved.Data.Version != 1 || saved.Data.Loaded != 1 || !saved.Data.Active || !strings.Contains(string(saved.Data.Rules), "v1") {
		t.Fatal("saved/loaded time configuration was not reported accurately")
	}
	if got := setting.TimePricingAt("gpt-4o", time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)); got.Multiplier != 2 {
		t.Fatal("time schedule was not loaded")
	}
	if after := relay.ComputeUsageQuota(relay.NewPriceData("gpt-4o", "default"), usage); after != before*2 {
		t.Fatal("loaded time rules did not activate consistently")
	}
	response := doRequest(env.base, "GET", "/api/option/pricing", env.session, nil)
	var prices struct {
		Data struct {
			Version int64          `json:"version"`
			Options []model.Option `json:"options"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(response.Body), &prices); err != nil || response.Status != 200 || prices.Data.Version != 1 || len(prices.Data.Options) != 6 {
		t.Fatal("existing six-key pricing snapshot compatibility changed")
	}
	if response := doRequest(env.base, "PUT", "/api/option/pricing", env.session, map[string]any{"expected_version": 1, "options": []model.Option{{Key: "cache_ratio", Value: "0.25"}}}); response.Status != 200 {
		t.Fatal("independent price save failed")
	}
	if response := doRequest(env.base, "PUT", "/api/option/time-pricing", env.session, map[string]any{"expected_version": 1, "rules": rules("stale")}); response.Status != 409 {
		t.Fatal("independent price edit did not invalidate time draft")
	}
	if err := model.UpsertOption("group_group_ratio", "corrupt"); err != nil {
		t.Fatal(err)
	}
	response = doRequest(env.base, "PUT", "/api/option/time-pricing", env.session, map[string]any{"expected_version": 2, "rules": rules("v3")})
	if response.Status != 500 || !strings.Contains(response.Body, "saved, but serving settings could not be reloaded") {
		t.Fatalf("unknown reload outcome: %d %s", response.Status, response.Body)
	}
	if state := read(); state.Data.Version != 3 || state.Data.Loaded != 2 || !strings.Contains(string(state.Data.Rules), "v3") {
		t.Fatal("committed and loaded versions were conflated")
	}
	if err := model.UpsertOption("group_group_ratio", "{}"); err != nil {
		t.Fatal(err)
	}
	if response := doRequest(env.base, "POST", "/api/option/reload", env.session, nil); response.Status != 200 {
		t.Fatal("explicit repair reload failed")
	}
	if root, err := model.GetUserByID(env.rootID); err != nil || root.Quota != 100_000_000 {
		t.Fatal("time configuration changed quota")
	}
}

package main

import (
	"encoding/json"
	"strings"
	"testing"

	"starhub/model"
	"starhub/setting"
)

func TestGroupConfigurationRoutesKeepUnrelatedPricesAndApplyExplicitDisable(t *testing.T) {
	env := setupRelayApp(t)
	if _, err := model.SaveOption(setting.OptionKeyGroupRatio, `{"default":1,"legacy-only":0}`, 7); err != nil {
		t.Fatal(err)
	}
	put := func(id, rate string, enabled bool) {
		t.Helper()
		res := doRequest(env.base, "PUT", "/api/service-group", env.session, map[string]any{"group_id": id, "name": id, "multiplier": rate, "enabled": enabled})
		if res.Status != 200 {
			t.Fatalf("group save: %d %s", res.Status, res.Body)
		}
	}
	put("managed", "0.8", true)
	if value, present := setting.GroupRatioValue("legacy-only"); !present || value != 0 {
		t.Fatal("legacy zero was removed from serving prices")
	}
	if err := model.DB.Model(&model.Token{}).Where("id = ?", env.tokenID).Update("unlimited_quota", true).Error; err != nil {
		t.Fatal(err)
	}
	before, err := model.GetUserByID(env.rootID)
	if err != nil {
		t.Fatal(err)
	}
	put("default", "1", false)
	if setting.ContainsGroupRatio("default") {
		t.Fatal("disabled default silently reenabled")
	}
	res := doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, map[string]any{"model": "gpt-4o", "messages": []any{map[string]any{"role": "user", "content": "must not forward"}}})
	if res.Status != 403 || !strings.Contains(res.Body, "group_not_available") {
		t.Fatalf("disabled group reached relay: %d %s", res.Status, res.Body)
	}
	put("managed", "0.6", true)
	if setting.ContainsGroupRatio("default") {
		t.Fatal("unrelated edit restored disabled group")
	}
	put("default", "1", true)
	if !setting.ContainsGroupRatio("default") {
		t.Fatal("management could not explicitly restore default")
	}
	res = doRequest(env.base, "DELETE", "/api/service-group/managed", env.session, nil)
	if res.Status != 200 || setting.ContainsGroupRatio("managed") {
		t.Fatalf("delete not applied: %d %s", res.Status, res.Body)
	}
	changes, err := model.ListOptionChanges(100)
	if err != nil || changes[0].ActorID != env.rootID {
		t.Fatal("option history did not retain actual actor")
	}
	var ratios map[string]float64
	if err := json.Unmarshal([]byte(changes[0].NewValue), &ratios); err != nil {
		t.Fatal(err)
	}
	if value, present := ratios["legacy-only"]; !present || value != 0 {
		t.Fatal("history lost unrelated zero")
	}
	after, err := model.GetUserByID(env.rootID)
	if err != nil || before.Quota != after.Quota {
		t.Fatal("configuration changed user quota")
	}
	var logs int64
	if err := model.DB.Model(&model.Log{}).Count(&logs).Error; err != nil || logs != 0 {
		t.Fatal("disabled request produced a usage log")
	}
	var audits int64
	if err := model.DB.Model(&model.AuditLog{}).Where("path = ? AND method = ? AND status = 200", "/api/service-group", "PUT").Count(&audits).Error; err != nil || audits != 4 {
		t.Fatalf("group audit: %d %v", audits, err)
	}
}

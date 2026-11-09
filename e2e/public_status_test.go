package main

import (
	"strings"
	"testing"
	"time"

	"starhub/model"
	"starhub/setting"
)

func TestPublicStatusAnonymousRoutesAndConfigurationAuthorization(t *testing.T) {
	env := setupRelayApp(t)
	if _, err := model.SaveOption(setting.OptionKeyGroupRatio, `{"high":1,"medium":0.8,"low":0.1}`, env.rootID); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"high", "medium", "low"} {
		if err := model.UpsertServiceGroup(&model.ServiceGroupConfig{GroupID: id, Name: id, Enabled: true, Visibility: "common"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := model.DB.Create(&model.StandardModel{Model: "status-fixture", Name: "Status fixture", Vendor: "Fixture", Kind: "chat", Listed: true}).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		status, code := 1, ""
		if i >= 45 {
			status, code = 2, "CONNECT_FAIL"
		}
		if err := model.DB.Create(&model.Log{Model: "status-fixture", ServiceGroup: "high", CreatedAt: time.Now().Unix(), Status: status, ErrorCode: code}).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, session := range []string{"", env.session} {
		r := doRequest(env.base, "GET", "/api/status/models?q=status-fixture", session, nil)
		if r.Status != 200 || !strings.Contains(r.Body, `"state":"outage"`) || !strings.Contains(r.Body, `"success_rate":0.9`) {
			t.Fatal(r.Status, r.Body)
		}
		r = doRequest(env.base, "GET", "/api/public/home", session, nil)
		if r.Status != 200 || !strings.Contains(r.Body, "Status fixture") {
			t.Fatal(r.Status, r.Body)
		}
		r = doRequest(env.base, "GET", "/api/status/models/status-fixture/history", session, nil)
		if r.Status != 200 || !strings.Contains(r.Body, `"hours"`) || !strings.Contains(r.Body, `"success_rate":0.9`) {
			t.Fatal(r.Status, r.Body)
		}
	}
	if r := doRequest(env.base, "GET", "/api/service-quality", "", nil); r.Status != 401 {
		t.Fatal("private quality route exposed", r.Status)
	}
	if r := doRequest(env.base, "PUT", "/api/service-group", "", map[string]any{"group_id": "high", "name": "High", "multiplier": "1", "status_page_mode": "exclude"}); r.Status != 401 {
		t.Fatal("anonymous configuration allowed", r.Status)
	}
}

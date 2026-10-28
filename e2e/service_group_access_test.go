package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"starhub/common"
	"starhub/model"
	"starhub/setting"
)

func TestGroupAccountPolicyControlsRealRelayAndPublicCatalog(t *testing.T) {
	env := setupRelayApp(t)
	var privateHits, commonHits atomic.Int64
	upstream := func(counter *atomic.Int64) *httptest.Server {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			counter.Add(1)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"id":"local","object":"chat.completion","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
		}))
		t.Cleanup(server.Close)
		return server
	}
	for _, fixture := range []struct {
		group string
		hits  *atomic.Int64
	}{{"private", &privateHits}, {"default", &commonHits}} {
		server := upstream(fixture.hits)
		if err := model.InsertChannel(&model.Channel{Name: fixture.group, Type: 1, Key: "local-only", BaseURL: server.URL, Models: "gpt-4o", Group: fixture.group, Status: common.ChannelStatusEnabled, Weight: 1}); err != nil {
			t.Fatal(err)
		}
	}
	if err := model.UpdateUserFields(env.rootID, map[string]any{"group": "private", "quota": int64(2_000_000)}); err != nil {
		t.Fatal(err)
	}
	if err := model.UpdateTokenFields(env.tokenID, map[string]any{"group": "private", "unlimited_quota": true}); err != nil {
		t.Fatal(err)
	}
	group := &model.ServiceGroupConfig{GroupID: "private", Name: "Private", Enabled: true, Visibility: "custom", AccountIDs: "acct-1,01,+1", Multiplier: "0.5"}
	if err := model.UpsertServiceGroup(group); err != nil {
		t.Fatal(err)
	}
	latency := int64(100)
	if _, err := model.UpdateServiceGroupSLA(context.Background(), group.GroupID, group.ID, 0, &model.ServiceGroupSLATarget{P95LatencyMS: &latency}); err != nil {
		t.Fatal(err)
	}
	if err := setting.UpdateRatio("group_ratio", `{"default":1,"private":0.5}`); err != nil {
		t.Fatal(err)
	}
	type catalogModel struct {
		Available bool     `json:"is_available"`
		Groups    []string `json:"available_groups"`
		Protocols []string `json:"protocols"`
		Reason    string   `json:"unavailable_reason"`
	}
	checkCatalog := func(allowed bool) {
		t.Helper()
		for _, session := range []string{env.session, ""} {
			response := doRequest(env.base, "GET", "/api/plaza/models/gpt-4o?group=default&user_id="+strconv.FormatInt(env.rootID, 10), session, nil)
			var detail struct {
				Data struct {
					Group string                 `json:"group"`
					Model catalogModel           `json:"model"`
					SLA   *model.ServiceGroupSLA `json:"group_sla"`
				} `json:"data"`
			}
			if err := json.Unmarshal([]byte(response.Body), &detail); err != nil || response.Status != 200 {
				t.Fatalf("detail: %d %s %v", response.Status, response.Body, err)
			}
			foundPrivate := false
			for _, id := range detail.Data.Model.Groups {
				foundPrivate = foundPrivate || id == "private"
			}
			if foundPrivate != (allowed && session != "") {
				t.Fatalf("custom group disclosed as accessible: %s", response.Body)
			}
			if session != "" {
				if detail.Data.Group != "private" || detail.Data.Model.Available != allowed || (detail.Data.SLA != nil) != allowed {
					t.Fatalf("session scope mismatch: %s", response.Body)
				}
				if !allowed && (len(detail.Data.Model.Protocols) != 0 || detail.Data.Model.Reason != "group_not_allowed") {
					t.Fatalf("denied group protocols advertised: %s", response.Body)
				}
			}
		}
		response := doRequest(env.base, "GET", "/api/plaza/models?available_only=1", env.session, nil)
		var list struct {
			Data struct {
				Total int `json:"total"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(response.Body), &list); err != nil || response.Status != 200 || (list.Data.Total == 1) != allowed {
			t.Fatalf("available-only mismatch: %d %s %v", response.Status, response.Body, err)
		}
	}
	checkCatalog(false)
	before, err := model.GetUserByID(env.rootID)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/v1/chat/completions", "/v1/messages", "/v1/responses", "/v1beta/models/gpt-4o:generateContent"} {
		response := doRequest(env.base, "POST", path, env.apiKey, map[string]any{"model": "gpt-4o"})
		if response.Status != 403 || !strings.Contains(response.Body, "cannot access the configured group") {
			t.Fatalf("unauthorized request: %s %d %s", path, response.Status, response.Body)
		}
	}
	after, err := model.GetUserByID(env.rootID)
	var logs int64
	if err != nil || after.Quota != before.Quota || after.UsedQuota != before.UsedQuota || privateHits.Load() != 0 || commonHits.Load() != 0 {
		t.Fatal("denied request reached an upstream or changed account usage")
	}
	if err := model.DB.Model(&model.Log{}).Count(&logs).Error; err != nil || logs != 0 {
		t.Fatal("denied request created a usage log")
	}
	group.AccountIDs = strconv.FormatInt(env.rootID, 10)
	if err := model.UpsertServiceGroup(group); err != nil {
		t.Fatal(err)
	}
	checkCatalog(true)
	payload := map[string]any{"model": "gpt-4o", "messages": []any{map[string]string{"role": "user", "content": "local provider only"}}}
	if response := doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, payload); response.Status != 200 || privateHits.Load() != 1 {
		t.Fatalf("authorized request: %d %s", response.Status, response.Body)
	}
	group.AccountIDs = "unrelated"
	if err := model.UpsertServiceGroup(group); err != nil {
		t.Fatal(err)
	}
	if err := model.UpdateTokenFields(env.tokenID, map[string]any{"group": common.AutoGroup, "auto_groups": "private,default"}); err != nil {
		t.Fatal(err)
	}
	if response := doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, payload); response.Status != 200 || privateHits.Load() != 1 || commonHits.Load() != 1 {
		t.Fatalf("auto group did not exclude revoked access: %d %s", response.Status, response.Body)
	}
	group.Enabled = false
	group.AccountIDs = strconv.FormatInt(env.rootID, 10)
	if err := model.UpsertServiceGroup(group); err != nil {
		t.Fatal(err)
	}
	if err := model.UpdateTokenFields(env.tokenID, map[string]any{"group": "private"}); err != nil {
		t.Fatal(err)
	}
	if response := doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, payload); response.Status != 403 || privateHits.Load() != 1 {
		t.Fatalf("live ratio bypassed disabled definition: %d %s", response.Status, response.Body)
	}
	if response := doRequest(env.base, "GET", "/api/service-group/configuration", env.session, nil); response.Status != 200 {
		t.Fatalf("restricted relay scope blocked management recovery: %d %s", response.Status, response.Body)
	}
}

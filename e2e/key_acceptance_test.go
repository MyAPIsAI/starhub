package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"starhub/model"
)

func keyActionFixture(t *testing.T, env p1Env, owner int64, action string, id int64, payload map[string]any) map[string]any {
	t.Helper()
	choices, err := model.ReadOwnedTokenChoices(owner)
	if err != nil {
		t.Fatal(err)
	}
	revision := strings.Repeat("0", 64)
	if token, err := model.GetTokenByID(id); err == nil {
		revision = model.TokenManagementRevision(token)
	}
	return map[string]any{"schema": "owned_token_action_v1", "expected_user_id": owner, "operation_id": strings.Repeat("a", 32), "action": action, "token_id": id, "revision": revision, "choices_revision": choices.Revision, "payload": payload}
}

func TestKeyAC17RejectsBothCreationPathsAtAccountLimit(t *testing.T) {
	env := setupRelayApp(t)
	owner, session := exportTestUser(t, env.base, "key-limit")
	if err := model.InsertChannel(&model.Channel{Name: "key choices", Type: 1, Status: 1, Key: "synthetic", Models: "alpha", Group: "default"}); err != nil {
		t.Fatal(err)
	}
	rows := make([]model.Token, model.MaxTokensPerUser)
	for i := range rows {
		rows[i] = model.Token{UserID: owner, Key: fmt.Sprintf("synthetic-cap-%d", i), Status: 2, ExpiredTime: 1}
	}
	if err := model.DB.CreateInBatches(rows, 100).Error; err != nil {
		t.Fatal(err)
	}
	fields := map[string]any{"name": "over limit", "group": "default", "model_limits": "alpha"}
	for _, request := range []struct {
		path string
		body any
	}{{"/api/token", fields}, {"/api/token/owned-action", keyActionFixture(t, env, owner, "create", 0, fields)}} {
		response := doRequest(env.base, "POST", request.path, session, request.body)
		if response.Status != 409 || !strings.Contains(response.Body, "maximum 200 keys") {
			t.Fatalf("cap not reported at %s: %d %s", request.path, response.Status, response.Body)
		}
	}
	var count, receipts int64
	model.DB.Model(&model.Token{}).Where("user_id = ?", owner).Count(&count)
	model.DB.Model(&model.OwnedTokenReceipt{}).Where("user_id = ?", owner).Count(&receipts)
	if count != model.MaxTokensPerUser || receipts != 0 {
		t.Fatalf("rejected create wrote state: keys=%d receipts=%d", count, receipts)
	}
}

func TestARCAC11ForeignKeysAreIndistinguishableFromMissingKeys(t *testing.T) {
	env := setupRelayApp(t)
	owner, session := exportTestUser(t, env.base, "key-privacy")
	before, err := model.GetTokenByID(env.tokenID)
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range []string{"view", "usage", "edit", "disable", "regenerate", "delete", "verify", "owned-update", "owned-regenerate", "owned-delete"} {
		t.Run(operation, func(t *testing.T) {
			responses := []apiResult{}
			for _, id := range []int64{env.tokenID, 99999999} {
				method, path, payload := "GET", fmt.Sprintf("/api/token/%d", id), any(nil)
				switch operation {
				case "usage":
					path += "/usage"
				case "edit", "disable":
					method, path = "PUT", "/api/token"
					payload = map[string]any{"id": id, "name": "must not change"}
					if operation == "disable" {
						payload = map[string]any{"id": id, "status": 2}
					}
				case "regenerate":
					method, path = "POST", path+"/regenerate"
				case "delete":
					method = "DELETE"
				case "verify":
					method, path = "POST", path+"/verify-password"
					payload = map[string]any{"expected_user_id": owner, "password": "synthetic-password"}
				case "owned-update", "owned-regenerate", "owned-delete":
					method, path = "POST", "/api/token/owned-action"
					fields := map[string]any{}
					if operation == "owned-update" {
						fields["name"] = "must not change"
					}
					payload = keyActionFixture(t, env, owner, strings.TrimPrefix(operation, "owned-"), id, fields)
				}
				response := doRequest(env.base, method, path, session, payload)
				if response.Status != 404 {
					t.Errorf("%s id=%d: %d %s", operation, id, response.Status, response.Body)
				}
				responses = append(responses, response)
			}
			if responses[0].Body != responses[1].Body {
				t.Fatal("response disclosed whether the foreign key exists")
			}
		})
	}
	after, err := model.GetTokenByID(env.tokenID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("foreign request changed the key or its usage")
	}
	if response := doRequest(env.base, "GET", fmt.Sprintf("/api/token/%d", env.tokenID), env.session, nil); response.Status != 200 || strings.Contains(response.Body, before.Key) {
		t.Fatalf("owner read unavailable or secret exposed: %d", response.Status)
	}
	if response := doRequest(env.base, "GET", fmt.Sprintf("/api/token/%d/usage", env.tokenID), env.session, nil); response.Status != 200 {
		t.Fatal("owner usage unavailable")
	}
}

func TestKeyAC14CandidatesRemainWithinAuthorizationAcrossAPIs(t *testing.T) {
	env := setupRelayApp(t)
	upstream := newMockUpstream(t)
	if err := model.InsertChannel(&model.Channel{Name: "smart policy", Type: 1, Status: 1, Key: upstreamKey, BaseURL: upstream.URL, Models: "alpha,beta,gamma", Group: "default"}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/token", "/api/token/owned-action"} {
		fields := map[string]any{"name": "invalid candidate", "group": "default", "model_limits": "alpha,beta", "smart_access": true, "smart_candidates": "gamma"}
		var payload any = fields
		if path == "/api/token/owned-action" {
			payload = keyActionFixture(t, env, env.rootID, "create", 0, fields)
		}
		if response := doRequest(env.base, "POST", path, env.session, payload); response.Status != 400 {
			t.Fatalf("outside candidate accepted at %s: %d %s", path, response.Status, response.Body)
		}
	}
	response := doRequest(env.base, "POST", "/api/token/owned-action", env.session, keyActionFixture(t, env, env.rootID, "create", 0, map[string]any{"name": "restricted smart", "group": "default", "model_limits": "alpha,beta", "smart_access": true, "smart_candidates": "beta", "unlimited_quota": true}))
	data := publicationAPIData[struct {
		Token struct {
			ID  int64  `json:"id"`
			Key string `json:"key"`
		} `json:"token"`
	}](t, response)
	for _, requested := range []string{"myapis-auto", "veyra-auto", "alpha"} {
		response := doRequest(env.base, "POST", "/v1/chat/completions", data.Token.Key, map[string]any{"model": requested, "messages": []map[string]string{{"role": "user", "content": "hello"}}, "max_tokens": 8})
		want := "beta"
		if requested == "alpha" {
			want = "alpha"
		}
		var result struct {
			Model string `json:"model"`
		}
		if err := json.Unmarshal([]byte(response.Body), &result); err != nil || response.Status != 200 || result.Model != want {
			t.Fatalf("smart policy not enforced: requested=%s status=%d body=%s", requested, response.Status, response.Body)
		}
	}
	if response := doRequest(env.base, "PUT", "/api/token", env.session, map[string]any{"id": data.Token.ID, "model_limits": "alpha"}); response.Status != 400 {
		t.Fatal("narrowing authorization retained an outside smart candidate")
	}
	if response := doRequest(env.base, "PUT", "/api/token", env.session, map[string]any{"id": data.Token.ID, "smart_access": false}); response.Status != 200 {
		t.Fatal("could not disable smart routing")
	}
	for _, requested := range []string{"myapis-auto", "veyra-auto", "alpha"} {
		response := doRequest(env.base, "POST", "/v1/chat/completions", data.Token.Key, map[string]any{"model": requested, "messages": []map[string]string{{"role": "user", "content": "hello"}}, "max_tokens": 8})
		want := 403
		if requested == "alpha" {
			want = 200
		}
		if response.Status != want {
			t.Fatalf("independent smart access: requested=%s status=%d want=%d", requested, response.Status, want)
		}
	}
}

package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"starhub/common"
	"starhub/model"
)

// This is a contract rehearsal with explicit fixture funding, not the timed manual UI-AC-13 acceptance.
func TestUIAC13QuickstartContract(t *testing.T) {
	env := setupRelayApp(t)
	upstream := newMockUpstream(t)
	if err := model.InsertChannel(&model.Channel{Name: "docs fixture", Type: 1, Status: 1, Key: upstreamKey, BaseURL: upstream.URL, Models: "gpt-4o-mini", Group: "default"}); err != nil {
		t.Fatal(err)
	}
	if common.DefaultRegisterQuota != 0 {
		t.Fatal("quickstart must not depend on trial quota")
	}
	registration := doRequest(env.base, "POST", "/api/user/register", "", map[string]string{"username": "docs-new-user", "password": "docs-fixture-password"})
	if registration.Status != 200 {
		t.Fatalf("registration: %d %s", registration.Status, registration.Body)
	}
	user, err := model.GetUserByUsername("docs-new-user")
	if err != nil || user.Quota != 0 {
		t.Fatal("registration did not preserve zero quota")
	}
	login := doRequest(env.base, "POST", "/api/user/login", "", map[string]string{"username": user.Username, "password": "docs-fixture-password"})
	data := publicationAPIData[struct {
		Session string `json:"session"`
	}](t, login)
	key := publicationAPIData[struct {
		Token struct {
			Key string `json:"key"`
		} `json:"token"`
	}](t, doRequest(env.base, "POST", "/api/token/owned-action", data.Session,
		keyActionFixture(t, env, user.ID, "create", 0, map[string]any{"name": "first-call", "group": "default", "model_limits": "gpt-4o-mini", "unlimited_quota": true})))
	if key.Token.Key == "" {
		t.Fatal("missing key")
	}
	models := doRequest(env.base, "GET", "/v1/models", key.Token.Key, nil)
	if models.Status != 200 || !strings.Contains(models.Body, "gpt-4o-mini") {
		t.Fatalf("authorized model discovery: %d %s", models.Status, models.Body)
	}
	raw, err := os.ReadFile("../web/src/pages/docs/quickstart-request.json")
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	body["model"] = "gpt-4o-mini"
	denied := doRequest(env.base, "POST", "/v1/chat/completions", key.Token.Key, body)
	if denied.Status != 403 || !strings.Contains(denied.Body, "insufficient_quota") {
		t.Fatalf("zero-quota key must not call: %d %s", denied.Status, denied.Body)
	}
	// Only the isolated test database is funded; this is not a payment path.
	if err := model.IncreaseUserQuota(user.ID, 1_000_000); err != nil {
		t.Fatal(err)
	}
	response := doRequest(env.base, "POST", "/v1/chat/completions", key.Token.Key, body)
	var completion struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(response.Body), &completion); err != nil || response.Status != 200 || len(completion.Choices) == 0 || completion.Choices[0].Message.Content == "" {
		t.Fatalf("documented first-call response: %d %s", response.Status, response.Body)
	}
	after, err := model.GetUserByID(user.ID)
	if err != nil || after.Quota >= 1_000_000 {
		t.Fatal("successful call did not settle usage")
	}
	body["model"] = "not-authorized"
	if response := doRequest(env.base, "POST", "/v1/chat/completions", key.Token.Key, body); response.Status != 403 {
		t.Fatalf("model authorization lost: %d %s", response.Status, response.Body)
	}
}

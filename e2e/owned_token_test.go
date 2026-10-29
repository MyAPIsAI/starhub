package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"starhub/model"
	"testing"
)

func TestOwnedKeyHTTPReceiptsAndRootIsolation(t *testing.T) {
	env := setupRelayApp(t)
	owner, session := exportTestUser(t, env.base, "owned-key-http")
	if err := model.InsertChannel(&model.Channel{Name: "keys", Type: 1, Status: 1, Key: "synthetic-key", Models: "owned-native-model", Group: "default"}); err != nil {
		t.Fatal(err)
	}
	choices, err := model.ReadOwnedTokenChoices(owner)
	if err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{"schema": "owned_token_action_v1", "expected_user_id": owner, "operation_id": "11111111111111111111111111111111", "action": "create", "choices_revision": choices.Revision, "payload": map[string]any{"name": "Native owned", "group": "default", "model_limits": "owned-native-model", "unlimited_quota": true}}
	call := func(path, cookie string) (int, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(payload)
		req, _ := http.NewRequest("POST", env.base+path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session", Value: cookie})
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var body map[string]any
		if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		return res.StatusCode, body
	}
	if status, _ := call("/api/token/owned-action", env.session); status != 409 {
		t.Fatal("Root bypassed owned request")
	}
	status, result := call("/api/token/owned-action", session)
	if status != 200 {
		t.Fatalf("owned create: %d %+v", status, result)
	}
	data := result["data"].(map[string]any)
	if data["schema"] != "owned_token_receipt_v1" || data["token"] == nil {
		t.Fatal("missing native receipt")
	}
	if status, _ := call("/api/token/owned-action", session); status != 409 {
		t.Fatal("duplicate operation created another key")
	}
	status, result = call("/api/token/owned-receipt", session)
	if status != 200 || result["data"].(map[string]any)["token"] != nil {
		t.Fatal("receipt recovery exposed plaintext")
	}
	req, _ := http.NewRequest("GET", env.base+"/api/token/owned", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: env.session})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if bytes.Contains(body, []byte("Native owned")) {
		t.Fatal("Root personal list returned another user's key")
	}
	var count int64
	model.DB.Model(&model.Token{}).Where("user_id = ? AND name = ?", owner, "Native owned").Count(&count)
	if count != 1 {
		t.Fatal(fmt.Sprint("created key count ", count))
	}
}

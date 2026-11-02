package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"starhub/model"
)

func TestKey4PasswordRevealAuditExpiryAndRotation(t *testing.T) {
	env := setupRelayApp(t)
	path := fmt.Sprintf("/api/token/%d", env.tokenID)
	response := doRequest(env.base, "POST", path+"/verify-password", env.session, map[string]any{"expected_user_id": env.rootID, "password": "123456"})
	if response.Status != 200 {
		t.Fatal("password reveal failed")
	}
	var result struct {
		Data struct {
			Key       string
			Grant     string
			ExpiresAt int64 `json:"expires_at"`
		}
	}
	if err := json.Unmarshal([]byte(response.Body), &result); err != nil {
		t.Fatal(err)
	}
	if result.Data.Key != env.apiKey || len(result.Data.Grant) != 64 || result.Data.ExpiresAt > time.Now().Add(5*time.Minute).Unix() {
		t.Fatal("invalid password reveal contract")
	}
	read := func(grant string) apiResult {
		return doRequest(env.base, "POST", path+"/reveal", env.session, map[string]any{"expected_user_id": env.rootID, "grant": grant})
	}
	if got := read(result.Data.Grant); got.Status != 200 || !strings.Contains(got.Body, env.apiKey) {
		t.Fatal("authorized repeat reveal failed")
	}
	var audits int64
	if err := model.DB.Model(&model.AuditLog{}).Where("user_id = ? AND path IN ? AND status = 200", env.rootID, []string{path + "/verify-password", path + "/reveal"}).Count(&audits).Error; err != nil || audits != 2 {
		t.Fatal("each key view must be audited")
	}
	if got := read(strings.Repeat("0", 64)); got.Status != 403 || strings.Contains(got.Body, env.apiKey) {
		t.Fatal("invalid reveal authorization leaked key")
	}
	if err := model.DB.Model(&model.TokenRevealGrant{}).Where("token_id = ?", env.tokenID).Update("expires_at", time.Now().Add(-time.Second).Unix()).Error; err != nil {
		t.Fatal(err)
	}
	if got := read(result.Data.Grant); got.Status != 403 || strings.Contains(got.Body, env.apiKey) {
		t.Fatal("expired reveal authorization remained usable")
	}
	response = doRequest(env.base, "POST", path+"/verify-password", env.session, map[string]any{"expected_user_id": env.rootID, "password": "123456"})
	if response.Status != 200 {
		t.Fatal("reverification failed")
	}
	if err := json.Unmarshal([]byte(response.Body), &result); err != nil {
		t.Fatal(err)
	}
	if got := doRequest(env.base, "POST", path+"/regenerate", env.session, nil); got.Status != 200 {
		t.Fatal("rotation failed")
	}
	if got := read(result.Data.Grant); got.Status != 403 {
		t.Fatal("rotating a key did not revoke reveal authorization")
	}
	if got := doRequest(env.base, "GET", "/api/token/owned", env.session, nil); strings.Contains(got.Body, env.apiKey) || strings.Contains(got.Body, "enc:v1:") {
		t.Fatal("list exposed key storage")
	}
}

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"starhub/common"
	"starhub/model"
)

func playgroundGrant(t *testing.T, env p1Env) string {
	t.Helper()
	response := doRequest(env.base, "POST", fmt.Sprintf("/api/token/%d/verify-password", env.tokenID), env.session, map[string]any{"expected_user_id": env.rootID, "password": "123456"})
	var result struct{ Data struct{ Grant string } }
	if response.Status != 200 || json.Unmarshal([]byte(response.Body), &result) != nil || result.Data.Grant == "" {
		t.Fatal("password verification failed")
	}
	return result.Data.Grant
}

func playgroundCall(t *testing.T, env p1Env, session, secret, grant, id, path string, body any) (*http.Response, string) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", env.base+path, bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+secret)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-MyAPIs-Source", "playground")
	req.Header.Set("X-MyAPIs-Key-ID", id)
	req.Header.Set("X-MyAPIs-Key-Grant", grant)
	if session != "" {
		req.AddCookie(&http.Cookie{Name: "session", Value: session})
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response, string(data)
}

func TestARCAC17PlaygroundKeyTamperingNeverCallsOrCharges(t *testing.T) {
	env := setupRelayApp(t)
	other, session := exportTestUser(t, env.base, "playground-other")
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
	t.Cleanup(upstream.Close)
	if err := model.InsertChannel(&model.Channel{Name: "playground", Type: 1, Status: 1, Key: upstreamKey, BaseURL: upstream.URL, Models: "prd-model", Group: "default"}); err != nil {
		t.Fatal(err)
	}
	keyPRDConfigure(t, env, "0.05")
	grant := playgroundGrant(t, env)
	before, _ := model.GetUserByID(env.rootID)
	keyBefore, _ := model.GetTokenByID(env.tokenID)
	for _, item := range []struct{ name, session, id string }{
		{"foreign session", session, fmt.Sprint(env.tokenID)},
		{"foreign key ID", env.session, "99999999"},
		{"missing key ID", env.session, ""},
		{"missing session", "", fmt.Sprint(env.tokenID)},
	} {
		t.Run(item.name, func(t *testing.T) {
			response, body := playgroundCall(t, env, item.session, env.apiKey, grant, item.id, "/v1/chat/completions", keyPRDBody(true))
			if response.StatusCode != 404 || !strings.Contains(body, "key_not_found") {
				t.Fatalf("expected 404, got %d %s", response.StatusCode, body)
			}
		})
	}
	for _, suffix := range []string{"/verify-password", "/reveal"} {
		payload := map[string]any{"expected_user_id": other, "grant": grant}
		if suffix == "/verify-password" {
			payload = map[string]any{"expected_user_id": other, "password": "export-password"}
		}
		if response := doRequest(env.base, "POST", fmt.Sprintf("/api/token/%d%s", env.tokenID, suffix), session, payload); response.Status != 404 {
			t.Fatalf("foreign key retrieval: %d", response.Status)
		}
	}
	after, _ := model.GetUserByID(env.rootID)
	keyAfter, _ := model.GetTokenByID(env.tokenID)
	logs, _ := model.CountLogs(0)
	if calls.Load() != 0 || logs != 0 || after.Quota != before.Quota || keyAfter.UsedQuota != keyBefore.UsedQuota || keyAfter.RemainQuota != keyBefore.RemainQuota {
		t.Fatal("tampered playground request executed or charged")
	}
}

func TestUI33PlaygroundVerificationExpiryAndIPRestrictions(t *testing.T) {
	env := setupRelayApp(t)
	id := fmt.Sprint(env.tokenID)
	response, body := playgroundCall(t, env, env.session, env.apiKey, "", id, "/v1/chat/completions", keyPRDBody(true))
	if response.StatusCode != 403 || !strings.Contains(body, "key_verification_required") {
		t.Fatal("unverified playground call allowed")
	}
	grant := playgroundGrant(t, env)
	if err := model.DB.Model(&model.Token{}).Where("id = ?", env.tokenID).Update("allow_ips", "192.0.2.1").Error; err != nil {
		t.Fatal(err)
	}
	response, body = playgroundCall(t, env, env.session, env.apiKey, grant, id, "/v1/chat/completions", keyPRDBody(true))
	if response.StatusCode != 403 || !strings.Contains(body, "ip_not_allowed") {
		t.Fatalf("IP policy bypassed: %d %s", response.StatusCode, body)
	}
	if err := model.DB.Model(&model.TokenRevealGrant{}).Where("token_id = ?", env.tokenID).Update("expires_at", time.Now().Add(-time.Second).Unix()).Error; err != nil {
		t.Fatal(err)
	}
	response, body = playgroundCall(t, env, env.session, env.apiKey, grant, id, "/v1/chat/completions", keyPRDBody(true))
	if response.StatusCode != 403 || !strings.Contains(body, "key_verification_required") {
		t.Fatal("expired password window accepted")
	}
}

func TestUI33PlaygroundSettledStreamingMetadataAndPrivacy(t *testing.T) {
	for _, protocol := range []string{"OpenAI", "Anthropic"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%v", protocol, stream), func(t *testing.T) {
				env := setupRelayApp(t)
				keyPRDConfigure(t, env, "0.05")
				upstream := newMockUpstream(t)
				if err := model.InsertChannel(&model.Channel{Name: "playground", Type: 1, Status: 1, Key: upstreamKey, BaseURL: upstream.URL, Models: "prd-model", Group: "default"}); err != nil {
					t.Fatal(err)
				}
				grant := playgroundGrant(t, env)
				body := keyPRDBody(stream)
				const privatePrompt = "playground-private-content-do-not-persist"
				body["messages"] = []map[string]string{{"role": "user", "content": privatePrompt}}
				path := "/v1/chat/completions"
				if protocol == "Anthropic" {
					path = "/v1/messages"
				}
				before, _ := model.GetUserByID(env.rootID)
				response, output := playgroundCall(t, env, env.session, env.apiKey, grant, fmt.Sprint(env.tokenID), path, body)
				if response.StatusCode != 200 || !strings.Contains(output, "hello") {
					t.Fatalf("call failed: %d %s", response.StatusCode, output)
				}
				trace := response.Header.Get("X-Request-ID")
				if len(trace) != 64 {
					t.Fatal("missing server trace")
				}
				metadata := doRequest(env.base, "GET", "/api/playground/results/"+trace, env.session, nil)
				if metadata.Status != 200 {
					t.Fatalf("missing settled metadata: %d %s", metadata.Status, metadata.Body)
				}
				var result struct {
					Data struct {
						KeyID  int64  `json:"key_id"`
						Cost   string `json:"cost_nano"`
						TTFT   int64  `json:"ttft_ms"`
						Source string
						Actual string `json:"actual_model"`
					}
				}
				if json.Unmarshal([]byte(metadata.Body), &result) != nil || result.Data.KeyID != env.tokenID || result.Data.Cost != "50000000" || result.Data.Source != "playground" || result.Data.Actual != "prd-model" || result.Data.TTFT < 1 {
					t.Fatalf("incorrect metadata: %s", metadata.Body)
				}
				after, _ := model.GetUserByID(env.rootID)
				key, _ := model.GetTokenByID(env.tokenID)
				if before.Quota-after.Quota != int64(0.05*common.QuotaPerUSD) || key.UsedQuota != int64(0.05*common.QuotaPerUSD) {
					t.Fatal("settled usage differs from account/key usage")
				}
				var entry model.Log
				if err := model.DB.Where("trace_id = ?", trace).First(&entry).Error; err != nil {
					t.Fatal(err)
				}
				encoded, _ := json.Marshal(entry)
				if entry.RequestSource != "playground" || entry.Protocol != protocol || strings.Contains(string(encoded), privatePrompt) || strings.Contains(string(encoded), "hello") {
					t.Fatal("request content persisted or metadata lost")
				}
				_, other := exportTestUser(t, env.base, "metadata-other")
				if response := doRequest(env.base, "GET", "/api/playground/results/"+trace, other, nil); response.Status != 404 {
					t.Fatal("foreign request metadata disclosed")
				}
			})
		}
	}
}

package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"starhub/model"
)

func TestStoredResponseActualRouterReadsTheOriginalAccountWithoutBilling(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream%t", stream), func(t *testing.T) {
			env := setupRelayApp(t)
			body := `{"id":"resp_router","status":"completed","store":true,"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"local stored response"}]}],"extension":9007199254740993,"usage":{"input_tokens":10,"output_tokens":5}}`
			wire := body
			if stream {
				wire = "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":" + body + "}\n\n"
			}
			var posts, gets atomic.Int64
			var selected, retrieved atomic.Value
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					posts.Add(1)
					selected.Store(r.Header.Get("Authorization"))
					kind := "application/json"
					if stream {
						kind = "text/event-stream"
					}
					w.Header().Set("Content-Type", kind)
					_, _ = io.WriteString(w, wire)
					return
				}
				gets.Add(1)
				retrieved.Store(r.Header.Get("Authorization"))
				if r.Method != http.MethodGet || r.URL.Path != "/v1/responses/resp_router" {
					t.Error("retrieve dispatched a different resource")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, body)
			}))
			defer upstream.Close()
			id := createChannelID(t, env, map[string]any{"name": "Stored response route", "key": "router-first\nrouter-second", "base_url": upstream.URL, "models": "gpt-4o", "key_mode": "round_robin"})
			payload := fmt.Sprintf(`{"model":"gpt-4o","input":"local","store":true,"stream":%t,"max_output_tokens":32}`, stream)
			request, _ := http.NewRequest(http.MethodPost, env.base+"/v1/responses", strings.NewReader(payload))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer "+env.apiKey)
			client := &http.Client{Timeout: 5 * time.Second}
			response, err := client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			actual, err := io.ReadAll(response.Body)
			_ = response.Body.Close()
			state := response.Header.Get("X-StarHub-Response-Reference")
			if stream {
				state = response.Trailer.Get("X-StarHub-Response-Reference")
			}
			if err != nil || response.StatusCode != 200 || string(actual) != wire || state != "recorded" {
				t.Fatalf("actual create/wire/reference receipt: %d %q %s %v", response.StatusCode, state, actual, err)
			}
			logs, total, err := model.ListLogs(env.rootID, 1, 10, "")
			if err != nil || total != 1 {
				t.Fatal("create did not settle once")
			}
			fee := logs[0].Quota
			ch, err := model.GetChannelByID(id)
			if err != nil {
				t.Fatal(err)
			}
			ch.Key = "router-second\nrouter-first"
			ch.KeyMode = model.KeyModeRandom
			if err := model.UpdateChannel(ch); err != nil {
				t.Fatal(err)
			}
			setOption(t, env, "model_price", `{"gpt-4o":0.02}`)
			if err := model.DB.Model(&model.User{}).Where("id = ?", env.rootID).Update("quota", 0).Error; err != nil {
				t.Fatal(err)
			}
			if err := model.DB.Model(&model.Token{}).Where("id = ?", env.tokenID).Updates(map[string]any{"remain_quota": 0, "unlimited_quota": false}).Error; err != nil {
				t.Fatal(err)
			}
			result := doRequest(env.base, "GET", "/v1/responses/resp_router?include=reasoning.encrypted_content", env.apiKey, nil)
			if result.Status != 200 || result.Body != body || gets.Load() != 1 || posts.Load() != 1 || selected.Load() != retrieved.Load() {
				t.Fatalf("actual original-account retrieve: %d %s", result.Status, result.Body)
			}
			owner, err := model.GetUserByID(env.rootID)
			token, tokenErr := model.GetTokenByID(env.tokenID)
			logs, total, logErr := model.ListLogs(env.rootID, 1, 10, "")
			if err != nil || tokenErr != nil || logErr != nil || owner.Quota != 0 || owner.UsedQuota != fee || token.RemainQuota != 0 || token.UsedQuota != fee || total != 1 || logs[0].Quota != fee {
				t.Fatal("retrieval was repriced, charged again or altered history")
			}
		})
	}
}

func TestStoredResponseActualRouterNeverProbesUnobservedIDs(t *testing.T) {
	env := setupRelayApp(t)
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fmt.Fprint(w, `{"id":"resp_unobserved","status":"completed"}`)
	}))
	defer upstream.Close()
	createChannelID(t, env, map[string]any{"name": "No response probing", "key": "synthetic-reference-key", "base_url": upstream.URL, "models": "gpt-4o"})
	before, err := model.GetUserByID(env.rootID)
	if err != nil {
		t.Fatal(err)
	}
	if result := doRequest(env.base, "GET", "/v1/responses/resp_unobserved", env.apiKey, nil); result.Status != 404 {
		t.Fatalf("unobserved ID: %d %s", result.Status, result.Body)
	}
	if calls.Load() != 0 {
		t.Fatal("unknown response ID was probed through a shared provider account")
	}
	after, err := model.GetUserByID(env.rootID)
	if err != nil || after.Quota != before.Quota || after.UsedQuota != before.UsedQuota {
		t.Fatal("unobserved read moved quota")
	}
	var references int64
	if err := model.DB.Model(&model.ResponseReference{}).Count(&references).Error; err != nil || references != 0 {
		t.Fatal("failed retrieval invented a reference")
	}
}

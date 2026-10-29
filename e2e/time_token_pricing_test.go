package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
)

func independentHTTPRules(modelName string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{%q:{"timezone":"UTC","windows":[{"id":"absolute","label":"Absolute","days":[0,1,2,3,4,5,6],"start":"00:00","end":"24:00","prices":{"input_per_m":"1","output_per_m":"2","cache_read_per_m":"3","cache_write_per_m":"4","audio_input_per_m":"5","audio_output_per_m":"6"}}]}}`, modelName))
}

func TestIndependentTimePricingHTTPGuardsAndTaskAdmission(t *testing.T) {
	env := setupRelayApp(t)
	const path = "/api/option/time-token-pricing"
	initial := doRequest(env.base, "GET", path, env.session, nil)
	var state struct {
		Data struct {
			Version int64  `json:"version"`
			Schema  string `json:"schema"`
			Active  bool   `json:"billing_active"`
			Tasks   bool   `json:"asynchronous_tasks"`
			Stack   string `json:"multiplier_behavior"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(initial.Body), &state); err != nil || initial.Status != 200 || state.Data.Version != 0 || state.Data.Schema != "weekly_token_prices_v1" || !state.Data.Active || !state.Data.Tasks || state.Data.Stack != "stacked" {
		t.Fatalf("capability mismatch: %d %s %v", initial.Status, initial.Body, err)
	}
	for _, role := range []int{common.RoleUser, common.RoleFinance, 99, 101, common.RoleAdmin} {
		user := &model.User{Username: fmt.Sprintf("absolute-role-%d", role), Role: role, Status: common.StatusEnabled, AffCode: common.GenerateAffCode()}
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
			response := doRequest(env.base, method, path, session, map[string]any{"expected_version": 0, "rules": map[string]any{}})
			if response.Status != want {
				t.Fatalf("role%d %s: %d %s", role, method, response.Status, response.Body)
			}
		}
	}
	if response := doRequest(env.base, "GET", path, "", nil); response.Status != 401 {
		t.Fatal("anonymous configuration read")
	}
	for _, payload := range []map[string]any{
		{"rules": independentHTTPRules("gpt-4o")},
		{"expected_version": -1, "rules": independentHTTPRules("gpt-4o")},
		{"expected_version": 0, "rules": nil},
		{"expected_version": 0, "rules": independentHTTPRules("myapis-auto")},
		{"expected_version": 0, "rules": independentHTTPRules("gpt-4o"), "unknown": true},
	} {
		if response := doRequest(env.base, "PUT", path, env.session, payload); response.Status != 400 {
			t.Fatalf("invalid draft: %d %s", response.Status, response.Body)
		}
	}
	if response := doRequest(env.base, "PUT", "/api/option", env.session, map[string]any{"key": "model_time_token_prices", "value": string(independentHTTPRules("gpt-4o"))}); response.Status != 400 {
		t.Fatal("generic endpoint accepted unguarded token draft")
	}
	for _, version := range []int64{-1, 1<<53 - 1} {
		if response := doRequest(env.base, "PUT", "/api/option", env.session, map[string]any{"key": "model_time_token_prices", "value": string(independentHTTPRules("gpt-4o")), "expected_version": version}); response.Status != 400 {
			t.Fatal("generic endpoint accepted an invalid version fence")
		}
	}
	if response := doRequest(env.base, "PUT", path, env.session, map[string]any{"expected_version": 0, "rules": independentHTTPRules("gpt-4o")}); response.Status != 200 {
		t.Fatalf("save: %d %s", response.Status, response.Body)
	}
	if response := doRequest(env.base, "PUT", path, env.session, map[string]any{"expected_version": 0, "rules": map[string]any{}}); response.Status != 409 {
		t.Fatal("stale draft erased rules")
	}
	if response := doRequest(env.base, "PUT", "/api/option", env.session, map[string]any{"key": "model_price", "value": `{"gpt-4o":0.01}`}); response.Status != 400 {
		t.Fatalf("generic billing mode conflict: %d %s", response.Status, response.Body)
	}
	if response := doRequest(env.base, "PUT", "/api/option/pricing", env.session, map[string]any{"expected_version": 1, "options": []model.Option{{Key: "model_price_expr", Value: `{"gpt-4o":"0.01"}`}}}); response.Status != 400 {
		t.Fatalf("typed billing mode conflict: %d %s", response.Status, response.Body)
	}
	if version, err := model.OptionVersion(); err != nil || version != 1 {
		t.Fatal("rejected modes advanced option history")
	}
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fmt.Fprint(w, `{"id":"task-tariff","status":"queued"}`)
	}))
	defer upstream.Close()
	createChannelID(t, env, map[string]any{"name": "Absolute task guard", "key": upstreamKey, "base_url": upstream.URL, "models": "gpt-4o"})
	quote := doRequest(env.base, "GET", "/api/plaza/models/gpt-4o", env.session, nil)
	var detail struct {
		Data struct {
			Model struct {
				Pricing map[string]any `json:"pricing"`
			} `json:"model"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(quote.Body), &detail); err != nil || quote.Status != 200 {
		t.Fatalf("actual public quote: %d %s %v", quote.Status, quote.Body, err)
	}
	for index, key := range []string{"input_per_m", "output_per_m", "cache_read_per_m", "cache_write_per_m", "audio_input_per_m", "audio_output_per_m"} {
		if detail.Data.Model.Pricing[key] != float64(index+1) {
			t.Fatalf("public %s = %v", key, detail.Data.Model.Pricing[key])
		}
	}
	before, err := model.GetUserByID(env.rootID)
	if err != nil {
		t.Fatal(err)
	}
	response := doRequest(env.base, "POST", "/v1/video/generations", env.apiKey, map[string]any{"model": "gpt-4o", "prompt": "local", "seconds": 8})
	if response.Status != 200 || calls.Load() != 1 {
		t.Fatalf("task admission failed: %d %s calls%d", response.Status, response.Body, calls.Load())
	}
	after, err := model.GetUserByID(env.rootID)
	if err != nil || after.Quota >= before.Quota || after.UsedQuota != before.UsedQuota {
		t.Fatal("task reservation did not reconcile")
	}
	var tasks int64
	if err := model.DB.Model(&model.Task{}).Count(&tasks).Error; err != nil || tasks != 1 {
		t.Fatal("admitted task was not persisted")
	}
}

func TestIndependentTokenHTTPReservesHighestClassAndRefundsFreeTariffs(t *testing.T) {
	env := setupRelayApp(t)
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"six-classes","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"local"},"finish_reason":"stop"}],"usage":{"prompt_tokens":400,"completion_tokens":200,"prompt_tokens_details":{"cached_tokens":100,"cache_write_tokens":100,"audio_tokens":100},"completion_tokens_details":{"audio_tokens":100}}}`)
	}))
	defer upstream.Close()
	createChannelID(t, env, map[string]any{"name": "Absolute six classes", "key": upstreamKey, "base_url": upstream.URL, "models": "gpt-4o"})
	setRules := func(rules json.RawMessage) {
		t.Helper()
		version, err := model.OptionVersion()
		if err != nil {
			t.Fatal(err)
		}
		result := doRequest(env.base, "PUT", "/api/option/time-token-pricing", env.session, map[string]any{"expected_version": version, "rules": rules})
		if result.Status != 200 {
			t.Fatalf("rules: %d %s", result.Status, result.Body)
		}
	}
	setRules(independentHTTPRules("gpt-4o"))
	if err := model.DB.Model(&model.User{}).Where("id = ?", env.rootID).Update("quota", 100).Error; err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{"model": "gpt-4o", "messages": []map[string]string{{"role": "user", "content": "local"}}, "max_tokens": 32}
	denied := doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, payload)
	if denied.Status != 403 || !strings.Contains(denied.Body, "insufficient_quota") || calls.Load() != 0 {
		t.Fatalf("maximum-class reservation bypassed: %d %s calls%d", denied.Status, denied.Body, calls.Load())
	}
	if err := model.DB.Model(&model.User{}).Where("id = ?", env.rootID).Update("quota", 100_000_000).Error; err != nil {
		t.Fatal(err)
	}
	if result := doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, payload); result.Status != 200 {
		t.Fatalf("six classes: %d %s", result.Status, result.Body)
	}
	logs, total, err := model.ListLogs(env.rootID, 1, 10, "")
	if err != nil || total != 1 || logs[0].Quota != 1050 || logs[0].PromptTokens != 400 || logs[0].CompletionTokens != 200 {
		t.Fatalf("six class settlement: %+v %d %v", logs, total, err)
	}
	free := string(independentHTTPRules("gpt-4o"))
	for _, price := range []string{"1", "2", "3", "4", "5", "6"} {
		free = strings.ReplaceAll(free, `:"`+price+`"`, `:"0"`)
	}
	setRules(json.RawMessage(free))
	if result := doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, payload); result.Status != 200 {
		t.Fatalf("free classes: %d %s", result.Status, result.Body)
	}
	logs, total, err = model.ListLogs(env.rootID, 1, 10, "")
	if err != nil || total != 2 || logs[0].Quota != 0 || logs[1].Quota != 1050 || calls.Load() != 2 {
		t.Fatal("free tariff changed old bills or forwarded twice")
	}
	owner, err := model.GetUserByID(env.rootID)
	token, tokenErr := model.GetTokenByID(env.tokenID)
	if err != nil || tokenErr != nil || owner.Quota != 100_000_000-1050 || owner.UsedQuota != 1050 || token.UsedQuota != 1050 {
		t.Fatal("free request did not refund its minimum hold exactly")
	}
}

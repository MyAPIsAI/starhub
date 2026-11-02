package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"starhub/common"
	"starhub/model"
)

func keyPRDBody(stream bool) map[string]any {
	return map[string]any{"model": "prd-model", "stream": stream, "max_tokens": 8, "messages": []map[string]string{{"role": "user", "content": "acceptance"}}}
}

func keyPRDConfigure(t *testing.T, env p1Env, price string) {
	t.Helper()
	setOption(t, env, "model_price", fmt.Sprintf(`{"prd-model":%s}`, price))
	setOption(t, env, "token_rate_limit", "10000")
	setOption(t, env, "user_rate_limit", "10000")
	if err := model.UpdateUserFields(env.rootID, map[string]any{"quota": int64(100 * common.QuotaPerUSD)}); err != nil {
		t.Fatal(err)
	}
}

func keyPRDFixture(t *testing.T, env p1Env, limit, used int64, period string) {
	t.Helper()
	loc, _ := time.LoadLocation("Asia/Dubai")
	now := time.Now().In(loc)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).Unix()
	if err := model.DB.Model(&model.Token{}).Where("id = ?", env.tokenID).Updates(map[string]any{
		"quota_period": period, "quota_timezone": "Asia/Dubai", "quota_limit": limit,
		"used_quota": used, "period_used_quota": used, "remain_quota": limit - used,
		"period_started_at": start, "unlimited_quota": false,
	}).Error; err != nil {
		t.Fatal(err)
	}
}

func keyPRDExpectError(t *testing.T, response apiResult, code string) {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(response.Body), &body); err != nil {
		t.Fatalf("invalid error response: %s", response.Body)
	}
	if response.Status != 403 || body.Error.Code != code {
		t.Errorf("expected 403 %s; got %d %s", code, response.Status, body.Error.Code)
	}
}

func TestKeyPRDAC03TwoGroupPrices(t *testing.T) {
	env := setupRelayApp(t)
	keyPRDConfigure(t, env, "0.05")
	upstream := newMockUpstream(t)
	var total int64
	for _, fixture := range []struct {
		group, rate string
		quota       int64
	}{{"ultra", "1", 25000}, {"lite", "0.4", 10000}} {
		response := doRequest(env.base, "PUT", "/api/service-group", env.session, map[string]any{"group_id": fixture.group, "name": fixture.group, "multiplier": fixture.rate, "enabled": true})
		if response.Status != 200 {
			t.Fatal(response.Body)
		}
		createChannelID(t, env, map[string]any{"name": fixture.group, "key": upstreamKey, "base_url": upstream.URL, "models": "prd-model", "group": fixture.group})
		response = doRequest(env.base, "POST", "/api/token", env.session, map[string]any{"name": fixture.group, "group": fixture.group, "model_limits": "prd-model", "unlimited_quota": true})
		key := publicationAPIData[model.Token](t, response)
		response = doRequest(env.base, "POST", "/v1/chat/completions", key.Key, keyPRDBody(false))
		if response.Status != 200 {
			t.Fatal(response.Body)
		}
		stored, _ := model.GetTokenByID(key.ID)
		if stored.UsedQuota != fixture.quota {
			t.Errorf("%s charge=%d, expected=%d", fixture.group, stored.UsedQuota, fixture.quota)
		}
		total += fixture.quota
	}
	user, _ := model.GetUserByID(env.rootID)
	if user.Quota != 100*common.QuotaPerUSD-total {
		t.Errorf("account charge mismatch: %d", user.Quota)
	}
}

func TestKeyPRDAC06DailyPrecharge(t *testing.T) {
	env := setupRelayApp(t)
	keyPRDConfigure(t, env, "0.05")
	upstream := newMockUpstream(t)
	createChannelID(t, env, map[string]any{"name": "daily", "key": upstreamKey, "base_url": upstream.URL, "models": "prd-model", "group": "default"})
	keyPRDFixture(t, env, 5000000, 4995000, "daily")
	response := doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, keyPRDBody(false))
	keyPRDExpectError(t, response, "key_quota_exceeded")
	key, _ := model.GetTokenByID(env.tokenID)
	user, _ := model.GetUserByID(env.rootID)
	if key.PeriodUsedQuota != 4995000 || key.RemainQuota != 5000 || user.Quota != 100*common.QuotaPerUSD {
		t.Error("rejected precharge moved money")
	}
}

func TestKeyPRDAC13IndependentBalanceGate(t *testing.T) {
	env := setupRelayApp(t)
	keyPRDConfigure(t, env, "0.02")
	upstream := newMockUpstream(t)
	createChannelID(t, env, map[string]any{"name": "balance", "key": upstreamKey, "base_url": upstream.URL, "models": "prd-model", "group": "default"})
	keyPRDFixture(t, env, 5000000, 0, "daily")
	if err := model.UpdateUserFields(env.rootID, map[string]any{"quota": int64(0)}); err != nil {
		t.Fatal(err)
	}
	response := doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, keyPRDBody(false))
	keyPRDExpectError(t, response, "insufficient_balance")
	key, _ := model.GetTokenByID(env.tokenID)
	if key.RemainQuota != 5000000 || key.UsedQuota != 0 {
		t.Error("balance rejection changed key usage")
	}
}

func TestKeyPRDAC24ExactConcurrentRemainder(t *testing.T) {
	env := setupRelayApp(t)
	keyPRDConfigure(t, env, "0.02")
	upstream := newMockUpstream(t)
	createChannelID(t, env, map[string]any{"name": "concurrent", "key": upstreamKey, "base_url": upstream.URL, "models": "prd-model", "group": "default"})
	keyPRDFixture(t, env, 500000, 475000, "daily")
	if err := model.InsertLog(&model.Log{UserID: env.rootID, TokenID: env.tokenID, Model: "prd-model", Quota: 475000, Status: 1}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan apiResult, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, keyPRDBody(false))
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	success, rejected := 0, 0
	for response := range results {
		if response.Status == 200 {
			success++
		} else {
			rejected++
			keyPRDExpectError(t, response, "key_quota_exceeded")
		}
	}
	key, _ := model.GetTokenByID(env.tokenID)
	user, _ := model.GetUserByID(env.rootID)
	var ledger int64
	if err := model.DB.Model(&model.Log{}).Where("token_id = ?", key.ID).Select("COALESCE(SUM(quota), 0)").Scan(&ledger).Error; err != nil {
		t.Fatal(err)
	}
	t.Logf("success=%d rejected=%d period_used=%d remaining=%d ledger=%d", success, rejected, key.PeriodUsedQuota, key.RemainQuota, ledger)
	if success != 2 || rejected != 8 || key.PeriodUsedQuota != 495000 || key.RemainQuota != 5000 || ledger != 495000 || user.Quota != 100*common.QuotaPerUSD-20000 {
		t.Error("exact concurrent accounting failed")
	}
}

func TestKeyPRDAC25DisabledGroupAndRestoredKeyPolicy(t *testing.T) {
	env := setupRelayApp(t)
	keyPRDConfigure(t, env, "0.02")
	var hits atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"audit","choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":100,"completion_tokens":50}}`)
	}))
	defer upstream.Close()
	group := func(enabled bool) {
		response := doRequest(env.base, "PUT", "/api/service-group", env.session, map[string]any{"group_id": "standard", "name": "standard", "multiplier": "0.6", "enabled": enabled})
		if response.Status != 200 {
			t.Fatal(response.Body)
		}
	}
	group(true)
	createChannelID(t, env, map[string]any{"name": "standard", "key": upstreamKey, "base_url": upstream.URL, "models": "prd-model,other-model", "group": "standard"})
	if err := model.UpdateTokenFields(env.tokenID, map[string]any{"group": "standard", "model_limits": "prd-model"}); err != nil {
		t.Fatal(err)
	}
	keyPRDFixture(t, env, 500000, 0, "daily")
	group(false)
	response := doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, keyPRDBody(false))
	keyPRDExpectError(t, response, "tier_capacity_unavailable")
	key, _ := model.GetTokenByID(env.tokenID)
	user, _ := model.GetUserByID(env.rootID)
	if key.PeriodUsedQuota != 0 || key.RemainQuota != 500000 || user.Quota != 100*common.QuotaPerUSD || hits.Load() != 0 {
		t.Error("disabled group moved money or called upstream")
	}
	group(true)
	response = doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, keyPRDBody(false))
	if response.Status != 200 || hits.Load() != 1 {
		t.Errorf("restored group unusable: %d", response.Status)
	}
	body := keyPRDBody(false)
	body["model"] = "other-model"
	keyPRDExpectError(t, doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, body), "model_not_allowed")
	if err := model.UpdateTokenFields(env.tokenID, map[string]any{"allow_ips": "203.0.113.9"}); err != nil {
		t.Fatal(err)
	}
	keyPRDExpectError(t, doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, keyPRDBody(false)), "ip_not_allowed")
	if err := model.UpdateTokenFields(env.tokenID, map[string]any{"allow_ips": "", "remain_quota": int64(0)}); err != nil {
		t.Fatal(err)
	}
	keyPRDExpectError(t, doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, keyPRDBody(false)), "key_quota_exceeded")
	if hits.Load() != 1 {
		t.Error("restoring group bypassed key policy")
	}
}

func TestKeyPRDAC21StreamingAcceptedPrice(t *testing.T) {
	env := setupRelayApp(t)
	keyPRDConfigure(t, env, "0.05")
	setOption(t, env, "group_ratio", `{"default":1,"pro":0.8,"ultra":1}`)
	started, release := make(chan struct{}), make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Stream bool `json:"stream"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		if request.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"id\":\"audit\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"}}]}\n\n")
			w.(http.Flusher).Flush()
			close(started)
			<-release
			fmt.Fprint(w, "data: {\"id\":\"audit\",\"choices\":[],\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":50}}\n\ndata: [DONE]\n\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"audit","choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":100,"completion_tokens":50}}`)
	}))
	defer func() { close(release); upstream.Close() }()
	pro := createChannelID(t, env, map[string]any{"name": "pro", "key": upstreamKey, "base_url": upstream.URL, "models": "prd-model", "group": "pro"})
	ultra := createChannelID(t, env, map[string]any{"name": "ultra", "key": upstreamKey, "base_url": upstream.URL, "models": "prd-model", "group": "ultra"})
	if err := model.UpdateTokenFields(env.tokenID, map[string]any{"group": "pro", "unlimited_quota": true}); err != nil {
		t.Fatal(err)
	}
	pending := make(chan apiResult, 1)
	go func() { pending <- doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, keyPRDBody(true)) }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("stream never started")
	}
	response := doRequest(env.base, "PUT", "/api/token", env.session, map[string]any{"id": env.tokenID, "group": "ultra"})
	if response.Status != 200 {
		t.Fatal(response.Body)
	}
	response = doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, keyPRDBody(false))
	if response.Status != 200 {
		t.Fatal(response.Body)
	}
	release <- struct{}{}
	select {
	case response = <-pending:
	case <-time.After(5 * time.Second):
		t.Fatal("stream never completed")
	}
	if response.Status != 200 || !strings.Contains(response.Body, "[DONE]") {
		t.Errorf("stream interrupted: %d", response.Status)
	}
	logs, _, err := model.ListLogs(env.rootID, 1, 100, "")
	if err != nil || len(logs) != 2 {
		t.Fatalf("missing settlements: %v", err)
	}
	for _, entry := range logs {
		want := int64(20000)
		if entry.ChannelID == ultra {
			want = 25000
		} else if entry.ChannelID != pro {
			t.Errorf("unexpected resource: %d", entry.ChannelID)
		}
		if entry.Quota != want {
			t.Errorf("accepted streaming price changed: %d expected %d", entry.Quota, want)
		}
	}
}

func TestKeyPRD2MultiplierContract(t *testing.T) {
	for _, fixture := range []struct {
		name, value string
		expected    int
	}{{"free", "0", 200}, {"four decimals", "0.1234", 200}, {"five decimals", "0.12345", 400}, {"upper bound", "100", 200}, {"over upper bound", "100.0001", 400}} {
		t.Run(fixture.name, func(t *testing.T) {
			env := setupRelayApp(t)
			response := doRequest(env.base, "PUT", "/api/service-group", env.session, map[string]any{"group_id": "audit-group", "name": "Audit", "multiplier": fixture.value, "enabled": true})
			if response.Status != fixture.expected {
				t.Errorf("multiplier %s: got %d expected %d", fixture.value, response.Status, fixture.expected)
			}
		})
	}
}

func TestKeyPRD2FreshDefaultGroups(t *testing.T) {
	setupRelayApp(t)
	groups, err := model.ListServiceGroups()
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, group := range groups {
		found[group.GroupID] = true
	}
	for _, id := range []string{"ultra", "pro", "standard", "lite"} {
		if !found[id] {
			t.Errorf("fresh seed is missing %s", id)
		}
	}
}

func TestKeyPRD3NameContract(t *testing.T) {
	for _, path := range []string{"/api/token", "/api/token/owned-action"} {
		t.Run(path, func(t *testing.T) {
			for _, fixture := range []struct {
				label, name string
				expected    int
			}{{"empty", "", 400}, {"51 ASCII characters", strings.Repeat("a", 51), 400}, {"50 CJK characters", strings.Repeat("\u6d4b", 50), 200}} {
				t.Run(fixture.label, func(t *testing.T) {
					env := setupRelayApp(t)
					upstream := newMockUpstream(t)
					createChannelID(t, env, map[string]any{"name": "names", "key": upstreamKey, "base_url": upstream.URL, "models": "prd-model", "group": "default"})
					fields := map[string]any{"name": fixture.name, "group": "default", "model_limits": "prd-model", "smart_access": false}
					var payload any = fields
					if path == "/api/token/owned-action" {
						payload = keyActionFixture(t, env, env.rootID, "create", 0, fields)
					}
					response := doRequest(env.base, "POST", path, env.session, payload)
					if response.Status != fixture.expected {
						t.Errorf("name %s: got %d expected %d", fixture.label, response.Status, fixture.expected)
					}
				})
			}
		})
	}
}

func TestKeyPRDAC25DisabledGroupAcrossGatewaysWithStaleRatios(t *testing.T) {
	env := setupRelayApp(t)
	keyPRDConfigure(t, env, "0.02")
	upstream := newMockUpstream(t)
	createChannelID(t, env, map[string]any{"name": "shared standard", "key": upstreamKey, "base_url": upstream.URL, "models": "prd-model", "group": "standard"})
	if err := model.UpdateTokenFields(env.tokenID, map[string]any{"group": "standard", "model_limits": "prd-model"}); err != nil {
		t.Fatal(err)
	}
	gateways := []string{keyGatewayProcess(t), keyGatewayProcess(t)}
	for _, base := range gateways {
		if response := doRequest(base, "POST", "/v1/chat/completions", env.apiKey, keyPRDBody(false)); response.Status != 200 {
			t.Fatalf("initial gateway request: %d %s", response.Status, response.Body)
		}
	}
	beforeKey, _ := model.GetTokenByID(env.tokenID)
	beforeUser, _ := model.GetUserByID(env.rootID)
	response := doRequest(env.base, "PUT", "/api/service-group", env.session, map[string]any{"group_id": "standard", "name": "Standard", "multiplier": "0.6", "enabled": false})
	if response.Status != 200 {
		t.Fatal(response.Body)
	}
	for _, base := range gateways {
		keyPRDExpectError(t, doRequest(base, "POST", "/v1/chat/completions", env.apiKey, keyPRDBody(false)), "tier_capacity_unavailable")
	}
	afterKey, _ := model.GetTokenByID(env.tokenID)
	afterUser, _ := model.GetUserByID(env.rootID)
	if afterKey.RemainQuota != beforeKey.RemainQuota || afterKey.UsedQuota != beforeKey.UsedQuota || afterUser.Quota != beforeUser.Quota {
		t.Fatal("disabled group on a stale gateway changed balances")
	}
}

func TestKeyPRD2FreeGroupDoesNotCharge(t *testing.T) {
	env := setupRelayApp(t)
	keyPRDConfigure(t, env, "0.05")
	upstream := newMockUpstream(t)
	response := doRequest(env.base, "PUT", "/api/service-group", env.session, map[string]any{"group_id": "free-group", "name": "Free", "multiplier": "0", "enabled": true})
	if response.Status != 200 {
		t.Fatal(response.Body)
	}
	createChannelID(t, env, map[string]any{"name": "free", "key": upstreamKey, "base_url": upstream.URL, "models": "prd-model", "group": "free-group"})
	if err := model.UpdateTokenFields(env.tokenID, map[string]any{"group": "free-group"}); err != nil {
		t.Fatal(err)
	}
	beforeKey, _ := model.GetTokenByID(env.tokenID)
	beforeUser, _ := model.GetUserByID(env.rootID)
	response = doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, keyPRDBody(false))
	if response.Status != 200 {
		t.Fatal(response.Body)
	}
	afterKey, _ := model.GetTokenByID(env.tokenID)
	afterUser, _ := model.GetUserByID(env.rootID)
	logs, total, err := model.ListLogs(env.rootID, 1, 20, "")
	if err != nil || total != 1 || logs[0].Quota != 0 || afterKey.RemainQuota != beforeKey.RemainQuota || afterKey.UsedQuota != beforeKey.UsedQuota || afterUser.Quota != beforeUser.Quota {
		t.Fatalf("zero multiplier: total=%d err=%v key remaining=%d to %d usage=%d to %d account=%d to %d", total, err, beforeKey.RemainQuota, afterKey.RemainQuota, beforeKey.UsedQuota, afterKey.UsedQuota, beforeUser.Quota, afterUser.Quota)
	}
}

func TestKeyPRD2LegacyRatioAPIRejectsInvalidMultipliersAtomically(t *testing.T) {
	env := setupRelayApp(t)
	before, err := model.OptionVersion()
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{`{"default":100.0001}`, `{"default":0.12345}`, `{"default":0.100000000000000000001}`, `{"default":null}`} {
		response := doRequest(env.base, "PUT", "/api/option", env.session, map[string]any{"key": "group_ratio", "value": value})
		if response.Status != 400 {
			t.Errorf("invalid group multiplier accepted by legacy API: %d", response.Status)
		}
	}
	after, err := model.OptionVersion()
	if err != nil || after != before {
		t.Fatal("invalid option request changed the configuration revision")
	}
}

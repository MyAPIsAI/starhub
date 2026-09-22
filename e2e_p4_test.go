package main

import (
	"encoding/json"
	"strings"
	"testing"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
	"starhub/relay"
)

// TestP4TokenGovernance exercises the new token fields: ModelLimits,
// AllowIps, and AutoGroups.
func TestP4TokenGovernance(t *testing.T) {
	upstream := newMockUpstream(t)
	env := setupRelayApp(t)
	chat := func(key, model string) apiResult {
		return doRequest(env.base, "POST", "/v1/chat/completions", key,
			map[string]any{"model": model,
				"messages": []map[string]string{{"role": "user", "content": "hi"}}})
	}

	// Set up two groups: "default" and "vip".
	setOption(t, env, "group_ratio", `{"default":1,"vip":0.8}`)

	// Channel in the default group.
	createChannelID(t, env, map[string]any{
		"name": "default-ch", "key": upstreamKey, "base_url": upstream.URL,
		"models": "gpt-4o-mini", "group": "default",
	})

	// 1. ModelLimits: a token restricted to gpt-4o-mini only.
	res := doRequest(env.base, "POST", "/api/token/", env.session, map[string]any{
		"name": "model-limited", "model_limits": "gpt-4o-mini",
	})
	if res.Status != 200 {
		t.Fatalf("create model-limited token failed: %d %s", res.Status, res.Body)
	}
	var mlResp struct {
		Data struct {
			Key string `json:"key"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(res.Body), &mlResp)
	mlKey := mlResp.Data.Key

	// Allowed model works.
	res = chat(mlKey, "gpt-4o-mini")
	if res.Status != 200 {
		t.Fatalf("model-limited token should relay gpt-4o-mini: %d %s", res.Status, res.Body)
	}
	// Disallowed model is rejected at the distributor (before billing).
	res = chat(mlKey, "gpt-4o")
	if res.Status != 403 || !strings.Contains(res.Body, "model_not_allowed") {
		t.Fatalf("model not in whitelist should be 403: %d %s", res.Status, res.Body)
	}

	// 2. AutoGroups: a token that tries "vip" then "default".
	// Channel in the vip group (higher priority so it's tried first).
	createChannelID(t, env, map[string]any{
		"name": "vip-ch", "key": upstreamKey, "base_url": upstream.URL,
		"models": "gpt-4o-mini", "group": "vip", "priority": 10,
	})
	res = doRequest(env.base, "POST", "/api/token/", env.session, map[string]any{
		"name": "auto-token", "group": "auto", "auto_groups": "vip,default",
	})
	if res.Status != 200 {
		t.Fatalf("create auto token failed: %d %s", res.Status, res.Body)
	}
	var autoResp struct {
		Data struct {
			Key string `json:"key"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(res.Body), &autoResp)
	autoKey := autoResp.Data.Key

	res = chat(autoKey, "gpt-4o-mini")
	if res.Status != 200 {
		t.Fatalf("auto token should relay via vip group: %d %s", res.Status, res.Body)
	}

	// 3. Auto group without auto_groups is rejected.
	res = doRequest(env.base, "POST", "/api/token/", env.session, map[string]any{
		"name": "bad-auto", "group": "auto",
	})
	if res.Status != 400 {
		t.Fatalf("auto group without auto_groups should be 400: %d %s", res.Status, res.Body)
	}

	// 4. AutoGroups with an unknown group is rejected.
	res = doRequest(env.base, "POST", "/api/token/", env.session, map[string]any{
		"name": "bad-auto2", "group": "auto", "auto_groups": "vip,unknown-group",
	})
	if res.Status != 400 {
		t.Fatalf("auto_groups with unknown group should be 400: %d %s", res.Status, res.Body)
	}
}

// TestP4GroupGroupRatio verifies the 2D group x group matrix multiplier.
func TestP4GroupGroupRatio(t *testing.T) {
	upstream := newMockUpstream(t)
	env := setupRelayApp(t)
	chat := func(key string) apiResult {
		return doRequest(env.base, "POST", "/v1/chat/completions", key,
			map[string]any{"model": "gpt-4o-mini",
				"messages": []map[string]string{{"role": "user", "content": "hi"}}})
	}

	// default group_ratio = 1, vip = 0.8 (20% discount).
	setOption(t, env, "group_ratio", `{"default":1,"vip":0.8}`)
	// Matrix: a "default" user using the "vip" group gets a 0.5 multiplier
	// on top of the vip group ratio.
	setOption(t, env, "group_group_ratio", `{"default":{"vip":0.5}}`)

	createChannelID(t, env, map[string]any{
		"name": "ch", "key": upstreamKey, "base_url": upstream.URL,
		"models": "gpt-4o-mini", "group": "vip",
	})

	// Create a token in the vip group.
	res := doRequest(env.base, "POST", "/api/token/", env.session, map[string]any{
		"name": "vip-token", "group": "vip",
	})
	if res.Status != 200 {
		t.Fatalf("create vip token failed: %d %s", res.Status, res.Body)
	}
	var tokenResp struct {
		Data struct {
			ID int64 `json:"id"`
			Key string `json:"key"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(res.Body), &tokenResp)

	// The root user's group is "default". The using group is "vip".
	// Effective ratio = group_ratio("vip") * group_group_ratio("default","vip")
//                  = 0.8 * 0.5 = 0.4
	expected := relay.ComputeQuotaWithUserGroup("gpt-4o-mini", "vip", "default", 100, 50)
	if expected <= 0 {
		t.Fatalf("expected quota should be positive, got %d", expected)
	}
	// Sanity: the matrix multiplier makes it cheaper than the plain vip ratio.
	plainVip := relay.ComputeQuota("gpt-4o-mini", "vip", 100, 50)
	if expected >= plainVip {
		t.Fatalf("matrix should reduce the bill: %d vs plain %d", expected, plainVip)
	}

	res = chat(tokenResp.Data.Key)
	if res.Status != 200 {
		t.Fatalf("vip relay failed: %d %s", res.Status, res.Body)
	}
	token, _ := model.GetTokenByID(tokenResp.Data.ID)
	if token.RemainQuota != common.DefaultTokenQuota-expected {
		t.Fatalf("token remain = %d, want %d (matrix applied)", token.RemainQuota,
			common.DefaultTokenQuota-expected)
	}
}

// TestP4RedemptionAndAffCode exercises the commercialization features:
// redemption codes, affiliate codes, and the rebate flow.
func TestP4RedemptionAndAffCode(t *testing.T) {
	env := setupRelayApp(t)

	// 1. Admin creates a redemption code.
	res := doRequest(env.base, "POST", "/api/redemption/", env.session, map[string]any{
		"quota": 500000, "count": 1,
	})
	if res.Status != 200 {
		t.Fatalf("create redemption failed: %d %s", res.Status, res.Body)
	}
	var rdResp struct {
		Data struct {
			Codes []string `json:"codes"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(res.Body), &rdResp)
	if len(rdResp.Data.Codes) != 1 {
		t.Fatalf("expected 1 code, got %d", len(rdResp.Data.Codes))
	}
	code := rdResp.Data.Codes[0]
	if !strings.HasPrefix(code, "RD-") {
		t.Fatalf("redemption code should start with RD-: %q", code)
	}

	// 2. Admin lists redemption codes.
	res = doRequest(env.base, "GET", "/api/redemption/", env.session, nil)
	if res.Status != 200 || !strings.Contains(res.Body, code) {
		t.Fatalf("redemption list should contain the code: %d %s", res.Status, res.Body)
	}

	// 3. Root user redeems the code.
	beforeQuota, _ := model.GetUserByID(env.rootID)
	res = doRequest(env.base, "POST", "/api/user/redeem", env.session, map[string]any{
		"code": code,
	})
	if res.Status != 200 {
		t.Fatalf("redeem failed: %d %s", res.Status, res.Body)
	}
	afterQuota, _ := model.GetUserByID(env.rootID)
	if afterQuota.Quota != beforeQuota.Quota+500000 {
		t.Fatalf("quota after redeem = %d, want %d", afterQuota.Quota, beforeQuota.Quota+500000)
	}

	// 4. Reusing a code fails.
	res = doRequest(env.base, "POST", "/api/user/redeem", env.session, map[string]any{
		"code": code,
	})
	if res.Status != 404 {
		t.Fatalf("reusing a code should fail: %d %s", res.Status, res.Body)
	}

	// 5. Root has an aff_code (auto-generated at seed time).
	root, _ := model.GetUserByID(env.rootID)
	if root.AffCode == "" {
		t.Fatal("root should have an aff_code")
	}

	// 6. Register with the aff code → the invitee's inviter_id is set.
	res = doRequest(env.base, "POST", "/api/user/register", "", map[string]string{
		"username": "invitee", "password": "invitee-pass",
		"email": "invitee@example.com", "aff_code": root.AffCode,
	})
	if res.Status != 200 {
		t.Fatalf("register invitee failed: %d %s", res.Status, res.Body)
	}
	var regResp struct {
		Data struct {
			ID        int64 `json:"id"`
			InviterID int64 `json:"inviter_id"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(res.Body), &regResp)
	if regResp.Data.InviterID != env.rootID {
		t.Fatalf("invitee inviter_id = %d, want %d", regResp.Data.InviterID, env.rootID)
	}

	// 7. Register bonus: set a bonus and register another invitee.
	setOption(t, env, "aff_register_bonus", `100000`)
	rootBefore, _ := model.GetUserByID(env.rootID)
	res = doRequest(env.base, "POST", "/api/user/register", "", map[string]string{
		"username": "invitee2", "password": "invitee2-pass",
		"aff_code": root.AffCode,
	})
	if res.Status != 200 {
		t.Fatalf("register invitee2 failed: %d %s", res.Status, res.Body)
	}
	rootAfter, _ := model.GetUserByID(env.rootID)
	if rootAfter.Quota != rootBefore.Quota+100000 {
		t.Fatalf("inviter register bonus = %d, want %d", rootAfter.Quota, rootBefore.Quota+100000)
	}

	// 8. Affiliate rebate: the inviter gets a cut of the invitee's spend.
	// aff_rebate_ratio defaults to 0.1 (10%).
	upstream := newMockUpstream(t)
	createChannelID(t, env, map[string]any{
		"name": "rebate-ch", "key": upstreamKey, "base_url": upstream.URL,
		"models": "gpt-4o-mini", "group": "default",
	})
	// Invitee2 logs in and creates a token.
	res = doRequest(env.base, "POST", "/api/user/login", "", map[string]string{
		"username": "invitee2", "password": "invitee2-pass",
	})
	if res.Status != 200 {
		t.Fatalf("invitee2 login failed: %d %s", res.Status, res.Body)
	}
	var loginResp struct {
		Data struct {
			Session string `json:"session"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(res.Body), &loginResp)
	invitee2Session := loginResp.Data.Session

	// Get invitee2's ID via self.
	res = doRequest(env.base, "GET", "/api/user/self", invitee2Session, nil)
	if res.Status != 200 {
		t.Fatalf("invitee2 self failed: %d %s", res.Status, res.Body)
	}
	var selfResp struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(res.Body), &selfResp)
	invitee2ID := selfResp.Data.ID

	_ = doRequest(env.base, "PUT", "/api/user/", env.session, map[string]any{
		"id": invitee2ID, "quota": 10000000,
	})

	res = doRequest(env.base, "POST", "/api/token/", invitee2Session, map[string]any{
		"name": "invitee2-key",
	})
	if res.Status != 200 {
		t.Fatalf("invitee2 create token failed: %d %s", res.Status, res.Body)
	}
	if res.Status != 200 {
		t.Fatalf("invitee2 create token failed: %d %s", res.Status, res.Body)
	}
	var tokResp struct {
		Data struct {
			Key string `json:"key"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(res.Body), &tokResp)

	// Invitee2 makes a relay call.
	rootBeforeRebate, _ := model.GetUserByID(env.rootID)
	chatRes := doRequest(env.base, "POST", "/v1/chat/completions", tokResp.Data.Key,
		map[string]any{"model": "gpt-4o-mini",
			"messages": []map[string]string{{"role": "user", "content": "hi"}}})
	if chatRes.Status != 200 {
		t.Fatalf("invitee2 relay failed: %d %s", chatRes.Status, chatRes.Body)
	}
	spend := relay.ComputeQuota("gpt-4o-mini", "default", 100, 50)
	expectedRebate := int64(float64(spend) * 0.1)
	rootAfterRebate, _ := model.GetUserByID(env.rootID)
	if rootAfterRebate.Quota != rootBeforeRebate.Quota+expectedRebate {
		t.Fatalf("inviter rebate = %d, want %d (spend %d, ratio 0.1)",
			rootAfterRebate.Quota-rootBeforeRebate.Quota, expectedRebate, spend)
	}
}

// TestP4AuditLog verifies that admin write operations leave an audit trail.
func TestP4AuditLog(t *testing.T) {
	env := setupRelayApp(t)

	// An admin write (update option) should produce an audit log.
	setOption(t, env, "group_ratio", `{"default":1,"vip":0.8}`)

	res := doRequest(env.base, "GET", "/api/audit/", env.session, nil)
	if res.Status != 200 {
		t.Fatalf("list audit logs failed: %d %s", res.Status, res.Body)
	}
	var auditResp struct {
		Data struct {
			Items []model.AuditLog `json:"items"`
			Total int64            `json:"total"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(res.Body), &auditResp)
	if auditResp.Data.Total == 0 {
		t.Fatal("audit log should contain at least one entry")
	}
	found := false
	for _, entry := range auditResp.Data.Items {
		if entry.Method == "PUT" && strings.Contains(entry.Path, "/api/option") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("audit log should contain the PUT /api/option entry: %+v", auditResp.Data.Items)
	}
}

// TestP4TokenRateLimit verifies the token-level rate limiter.
func TestP4TokenRateLimit(t *testing.T) {
	upstream := newMockUpstream(t)
	env := setupRelayApp(t)
	createChannelID(t, env, map[string]any{
		"name": "ch", "key": upstreamKey, "base_url": upstream.URL,
		"models": "gpt-4o-mini", "group": "default",
	})
	chat := func(key string) apiResult {
		return doRequest(env.base, "POST", "/v1/chat/completions", key,
			map[string]any{"model": "gpt-4o-mini",
				"messages": []map[string]string{{"role": "user", "content": "hi"}}})
	}

	// Create a token.
	res := doRequest(env.base, "POST", "/api/token/", env.session, map[string]any{"name": "rl-key"})
	if res.Status != 200 {
		t.Fatalf("create token failed: %d %s", res.Status, res.Body)
	}
	var tokenResp struct {
		Data struct {
			Key string `json:"key"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(res.Body), &tokenResp)
	key := tokenResp.Data.Key

	// Configure a tight token rate limit (2/min).
	middleware.ConfigureTokenRateLimit(2)
	middleware.ResetTokenRateLimits()
	t.Cleanup(func() {
		middleware.ConfigureTokenRateLimit(middleware.DefaultTokenLimitPerMinute)
		middleware.ResetTokenRateLimits()
	})

	for i := 0; i < 2; i++ {
		res := chat(key)
		if res.Status != 200 {
			t.Fatalf("relay call %d should pass, got %d %s", i, res.Status, res.Body)
		}
	}
	res = chat(key)
	if res.Status != 429 || !strings.Contains(res.Body, "rate_limit_error") {
		t.Fatalf("third call should hit the token rate limit: %d %s", res.Status, res.Body)
	}
}

// TestP4ModelRateLimit verifies the model-level rate limiter.
func TestP4ModelRateLimit(t *testing.T) {
	upstream := newMockUpstream(t)
	env := setupRelayApp(t)
	createChannelID(t, env, map[string]any{
		"name": "ch", "key": upstreamKey, "base_url": upstream.URL,
		"models": "gpt-4o-mini", "group": "default",
	})
	chat := func(key string) apiResult {
		return doRequest(env.base, "POST", "/v1/chat/completions", key,
			map[string]any{"model": "gpt-4o-mini",
				"messages": []map[string]string{{"role": "user", "content": "hi"}}})
	}

	middleware.ConfigureModelRateLimit(2)
	middleware.ResetModelRateLimits()
	t.Cleanup(func() {
		middleware.ConfigureModelRateLimit(middleware.DefaultModelLimitPerMinute)
		middleware.ResetModelRateLimits()
	})

	for i := 0; i < 2; i++ {
		res := chat(env.apiKey)
		if res.Status != 200 {
			t.Fatalf("relay call %d should pass, got %d %s", i, res.Status, res.Body)
		}
	}
	res := chat(env.apiKey)
	if res.Status != 429 {
		t.Fatalf("third call should hit the model rate limit: %d %s", res.Status, res.Body)
	}

	// A different model is not affected (different bucket).
	middleware.ResetModelRateLimits()
	res = doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey,
		map[string]any{"model": "gpt-4o",
			"messages": []map[string]string{{"role": "user", "content": "hi"}}})
	if res.Status == 429 {
		t.Fatal("a different model should not be limited by the gpt-4o-mini bucket")
	}
}

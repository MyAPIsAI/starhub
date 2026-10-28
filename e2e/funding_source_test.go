package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
)

func TestFundingSourceActualSimulationCannotCertifyFundsOrMintAPIQuota(t *testing.T) {
	env := setupRelayApp(t)
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fmt.Fprint(w, `{"choices":[],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	t.Cleanup(upstream.Close)
	createChannelID(t, env, map[string]any{"name": "Funding boundary", "type": 1, "key": "synthetic-funding-key", "base_url": upstream.URL, "models": "alpha", "group": "default", "status": 1})
	user := &model.User{Username: "unfunded-api-owner", Role: common.RoleUser, Status: common.StatusEnabled, Group: "default", Quota: 0, AffCode: common.GenerateAffCode()}
	if err := model.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	session := middleware.Sessions.Create(user.ID, time.Hour)
	t.Cleanup(func() { middleware.Sessions.Remove(session) })
	for _, claim := range []map[string]any{{"funding_source": "verified_provider"}, {"api_quota_eligible": true}, {"trusted": true}, {"provider_transaction_id": "invented"}} {
		body := map[string]any{"amount_usd": "10.00", "channel": "card"}
		for key, value := range claim {
			body[key] = value
		}
		if res := doRequest(env.base, "POST", "/api/recharge", session, body); res.Status != 400 {
			t.Fatalf("client funding claim accepted: %d %s", res.Status, res.Body)
		}
	}
	var orders int64
	if err := model.DB.Model(&model.RechargeOrder{}).Count(&orders).Error; err != nil || orders != 0 {
		t.Fatal("invalid funding claim created orders")
	}
	pending := publicationAPIData[map[string]any](t, doRequest(env.base, "POST", "/api/recharge", session, map[string]any{"amount_usd": "10.00", "channel": "card"}))
	if pending["funding_source"] != "simulation" || pending["api_quota_eligible"] != false {
		t.Fatalf("pending source: %+v", pending)
	}
	id := int64(pending["id"].(float64))
	resolvePath := "/api/recharge/" + strconv.FormatInt(id, 10) + "/resolve"
	if res := doRequest(env.base, "POST", resolvePath, session, map[string]any{"outcome": "success", "funding_source": "verified_provider"}); res.Status != 400 {
		t.Fatal("manual resolution certified funding source")
	}
	paid := publicationAPIData[map[string]any](t, doRequest(env.base, "POST", resolvePath, session, map[string]any{"outcome": "success"}))
	if paid["status"] != "paid" || paid["funding_source"] != "simulation" || paid["api_quota_eligible"] != false {
		t.Fatalf("paid source: %+v", paid)
	}
	publicationAPIData[any](t, doRequest(env.base, "POST", resolvePath, session, map[string]any{"outcome": "success"}))
	wallet := publicationAPIData[map[string]any](t, doRequest(env.base, "GET", "/api/wallet", session, nil))
	if wallet["balance_nano"] != "10000000000" || wallet["api_quota_conversion_available"] != false {
		t.Fatalf("wallet bridge invented: %+v", wallet)
	}
	ledger := publicationAPIData[[]map[string]any](t, doRequest(env.base, "GET", "/api/ledger", session, nil))
	if len(ledger) != 1 || ledger[0]["funding_source"] != "simulation" || ledger[0]["api_quota_eligible"] != false || ledger[0]["amount_nano"] != "10000000000" {
		t.Fatalf("ledger trust or repeated credit: %+v", ledger)
	}
	token := publicationAPIData[model.Token](t, doRequest(env.base, "POST", "/api/token", session, map[string]any{"name": "Unfunded API token", "group": "default", "unlimited_quota": true}))
	response := doRequest(env.base, "POST", "/v1/chat/completions", token.Key, map[string]any{"model": "alpha", "messages": []map[string]string{{"role": "user", "content": "local"}}, "max_tokens": 10})
	owner, err := model.GetUserByID(user.ID)
	stored, errWallet := model.GetOrCreateWallet(user.ID)
	if response.Status < 400 || calls.Load() != 0 || err != nil || errWallet != nil || owner.Quota != 0 || stored.Balance != 10_000_000_000 {
		t.Fatalf("simulated funds bypassed API funding: status%d calls%d quota%d wallet%d", response.Status, calls.Load(), owner.Quota, stored.Balance)
	}
	export := publicationAPIData[model.ExportRecord](t, doRequest(env.base, "POST", "/api/export", session, map[string]any{"type": "ledger", "range_days": 1}))
	csv := doRequest(env.base, "GET", "/api/export/"+strconv.FormatInt(export.ID, 10)+"/download", session, nil)
	if csv.Status != 200 || !strings.Contains(csv.Body, "Funding Source,API Quota Eligible") || !strings.Contains(csv.Body, ",simulation,false") {
		t.Fatalf("export lost funding source: %d %s", csv.Status, csv.Body)
	}
	legacy := model.LedgerEntry{UserID: user.ID, Type: "adjustment", Amount: 1, BalanceAfter: 10_000_000_001, Description: "Unverified fixture", FundingSource: "invented_verified"}
	if err := model.DB.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	ledger = publicationAPIData[[]map[string]any](t, doRequest(env.base, "GET", "/api/ledger", session, nil))
	if len(ledger) != 2 || ledger[0]["funding_source"] != "legacy_unverified" || ledger[0]["api_quota_eligible"] != false {
		t.Fatal("unknown persisted provenance was promoted")
	}
}

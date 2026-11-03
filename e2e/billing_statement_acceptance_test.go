package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"starhub/model"
)

func TestBILAC27RealUsageStatementAndMonthlyTotal(t *testing.T) {
	env := setupRelayApp(t)
	keyPRDConfigure(t, env, "0.001")
	setOption(t, env, "ip_rate_limit", "10000")
	setOption(t, env, "model_rate_limit", "10000")
	upstream := newMockUpstream(t)
	if err := model.InsertChannel(&model.Channel{Name: "statement", Type: 1, Status: 1, Key: upstreamKey, BaseURL: upstream.URL, Models: "prd-model", Group: "default"}); err != nil {
		t.Fatal(err)
	}
	if err := model.DB.Create(&[]model.LedgerEntry{{UserID: env.rootID, Type: "recharge", Amount: 10000000000, BalanceAfter: 10000000000, OrderID: "ord-a", Description: "First recharge", FundingSource: model.FundingSourceSimulation}, {UserID: env.rootID, Type: "recharge", Amount: 20000000000, BalanceAfter: 30000000000, OrderID: "ord-b", Description: "Second recharge", FundingSource: model.FundingSourceSimulation}, {UserID: env.rootID, Type: "adjustment", Amount: 1000000000, BalanceAfter: 31000000000, Description: "Adjustment"}}).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 300; i++ {
		response := doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, keyPRDBody(false))
		if response.Status != 200 {
			t.Fatalf("request %d: %d %s", i, response.Status, response.Body)
		}
	}
	response := doRequest(env.base, "GET", "/api/ledger?scope=owned", env.session, nil)
	var statement struct{ Data []model.LedgerEntry }
	if response.Status != 200 || json.Unmarshal([]byte(response.Body), &statement) != nil {
		t.Fatal("statement unavailable")
	}
	recharges, adjustments, usage := 0, 0, 0
	var debit int64
	for _, entry := range statement.Data {
		switch entry.Type {
		case "recharge":
			recharges++
			if entry.OrderID == "" {
				t.Fatal("recharge lost order link")
			}
		case "adjustment":
			adjustments++
		case "call_summary":
			usage++
			debit -= entry.Amount
			if entry.RequestCount != 300 || entry.TokenID != env.tokenID {
				t.Fatal("daily key summary lost request count")
			}
		}
	}
	if recharges != 2 || adjustments != 1 || usage != 1 || debit != 300000000 {
		t.Fatalf("incorrect statement: %+v", statement.Data)
	}
	wallet := doRequest(env.base, "GET", "/api/wallet", env.session, nil)
	var summary struct {
		Data struct {
			Spent   string `json:"month_spend_nano"`
			Balance string `json:"api_balance_nano"`
		}
	}
	if json.Unmarshal([]byte(wallet.Body), &summary) != nil || summary.Data.Spent != fmt.Sprint(debit) || summary.Data.Balance != "99700000000" {
		t.Fatalf("monthly total or available balance does not reconcile: %s", wallet.Body)
	}
	exported := doRequest(env.base, "POST", "/api/export", env.session, map[string]any{"type": "ledger", "range_days": 31, "language": "zh", "ledger_type": "recharge"})
	var export struct {
		Data struct {
			ID         int64
			Count      int    `json:"row_count"`
			URL        string `json:"download_url"`
			Dictionary string `json:"dictionary_url"`
		}
	}
	if json.Unmarshal([]byte(exported.Body), &export) != nil || export.Data.Count != 2 {
		t.Fatalf("filtered export differs from the two visible recharges: %s", exported.Body)
	}
	csv := doRequest(env.base, "GET", export.Data.URL, env.session, nil)
	if csv.Status != 200 || !strings.HasPrefix(csv.Body, "\ufeff") || !strings.Contains(csv.Body, "金额（美元）") || !strings.Contains(csv.Body, "10.000000000") {
		t.Fatal("localized exact-money export failed")
	}
	dictionary := doRequest(env.base, "GET", export.Data.Dictionary, env.session, nil)
	if dictionary.Status != 200 || !strings.Contains(dictionary.Body, "nano-USD") {
		t.Fatal("field descriptions unavailable")
	}
	_, other := exportTestUser(t, env.base, "statement-other")
	if foreign := doRequest(env.base, "GET", export.Data.URL, other, nil); foreign.Status != 404 {
		t.Fatal("foreign download did not return 404")
	}
}

func TestUI36RequestPageTotalAndExportUseIdenticalOwnerFilters(t *testing.T) {
	env := setupRelayApp(t)
	other, _ := exportTestUser(t, env.base, "request-page-other")
	logs := make([]model.Log, 375)
	for i := range logs {
		owner := env.rootID
		if i >= 325 {
			owner = other
		}
		status := 1
		if i >= 225 && i < 325 {
			status = 2
		}
		logs[i] = model.Log{UserID: owner, Model: "selected-model", Status: status, Quota: 1, TokenID: env.tokenID}
	}
	if err := model.DB.CreateInBatches(&logs, 100).Error; err != nil {
		t.Fatal(err)
	}
	var first struct {
		Data struct {
			Items []model.Log
			Total int
		}
	}
	response := doRequest(env.base, "GET", "/api/log?scope=owned&p=1&page_size=20&status=success&query=selected-model", env.session, nil)
	if response.Status != 200 || json.Unmarshal([]byte(response.Body), &first) != nil || first.Data.Total != 225 || len(first.Data.Items) != 20 {
		t.Fatalf("paged total lost rows or crossed owners: %s", response.Body)
	}
	var second struct {
		Data struct {
			Items []model.Log
			Total int
		}
	}
	response = doRequest(env.base, "GET", "/api/log?scope=owned&p=2&page_size=20&status=success&query=selected-model", env.session, nil)
	if json.Unmarshal([]byte(response.Body), &second) != nil || second.Data.Total != 225 || len(second.Data.Items) != 20 || second.Data.Items[0].ID == first.Data.Items[0].ID {
		t.Fatal("server pagination did not advance")
	}
	response = doRequest(env.base, "POST", "/api/export", env.session, map[string]any{"type": "requests", "range_days": 31, "status": "success", "query": "selected-model"})
	var file struct {
		Data struct {
			Count int `json:"row_count"`
		}
	}
	if response.Status != 200 || json.Unmarshal([]byte(response.Body), &file) != nil || file.Data.Count != first.Data.Total {
		t.Fatalf("page/export counts differ: %d %s", first.Data.Total, response.Body)
	}
}

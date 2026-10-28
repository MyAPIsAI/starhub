package controller

import (
	"encoding/json"
	"testing"

	"starhub/common"
)

func TestFinanceSummaryValidatesScopeAndDays(t *testing.T) {
	newTestDB(t)
	for _, role := range []int{0, common.RoleUser, 99} {
		if rec := call(GetFinanceSummary, testRequest{target: "/api/finance/summary", role: role}); rec.Code != 403 {
			t.Fatalf("role %d read finance: %d", role, rec.Code)
		}
	}
	for _, days := range []string{"0", "-1", "91", "1.5", "invalid"} {
		if rec := call(GetFinanceSummary, testRequest{target: "/api/finance/summary?days=" + days, role: common.RoleFinance}); rec.Code != 400 {
			t.Fatalf("invalid days %q accepted: %d", days, rec.Code)
		}
	}
	for _, role := range []int{common.RoleFinance, common.RoleAdmin, common.RoleRoot} {
		rec := call(GetFinanceSummary, testRequest{target: "/api/finance/summary?days=7", role: role})
		var body struct {
			Data struct {
				Since     int64  `json:"since"`
				Until     int64  `json:"until"`
				Wallet    string `json:"wallet_balance_nano"`
				Collected string `json:"api_collected_nano"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != 200 || body.Data.Until-body.Data.Since != 7*86400 || body.Data.Wallet != "0" || body.Data.Collected != "0" {
			t.Fatalf("finance response has wrong scope/unit/type: %d %s %v", rec.Code, rec.Body, err)
		}
	}
}

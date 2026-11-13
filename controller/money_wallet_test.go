package controller

import (
	"fmt"
	"strings"
	"testing"

	"starhub/common"
	"starhub/model"
)

func TestCanonicalWalletUsesSpendableQuotaAndNeverPromotesSimulation(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "canonical-wallet", common.RoleUser, 0)
	if err := model.DB.Create(&model.Wallet{UserID: u.ID, Balance: 9000000000}).Error; err != nil {
		t.Fatal(err)
	}
	var wallet map[string]any
	w := call(MoneyWallet, testRequest{target: "/api/wallet/v2", userID: u.ID, role: u.Role})
	decodeData(t, w, &wallet)
	if wallet["balance_nano"] != "0" || wallet["simulation_conversion_available"] != false || wallet["api_quota_linked"] != true || !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
		t.Fatalf("simulation promoted: %+v", wallet)
	}
	other := newUser(t, "foreign-money", common.RoleUser, 0)
	w = call(MoneyLedger, testRequest{target: fmt.Sprintf("/api/ledger/v2?user_id=%d", other.ID), userID: u.ID, role: u.Role})
	if w.Code != 403 {
		t.Fatal("foreign money ledger exposed")
	}
}

func TestMoneyAdjustmentHTTPGuardsOwnerPayloadAndRole(t *testing.T) {
	newTestDB(t)
	root := newUser(t, "wallet-admin", common.RoleRoot, 0)
	u := newUser(t, "wallet-recipient", common.RoleUser, 0)
	body := fmt.Sprintf(`{"schema":"money_adjustment_v2","expected_actor_id":%d,"user_id":%d,"operation_id":"http-grant","amount_nano":"1","expected_available_nano":"0","reason":"Verified admin funding"}`, root.ID, u.ID)
	for _, sample := range []struct {
		uid  int64
		role int
		body string
		want int
	}{{u.ID, u.Role, body, 403}, {root.ID, root.Role, strings.Replace(body, fmt.Sprintf(`"expected_actor_id":%d`, root.ID), `"expected_actor_id":999`, 1), 409}, {root.ID, root.Role, strings.TrimSuffix(body, "}") + `,"funding_source":"verified"}`, 400}, {root.ID, root.Role, body, 200}, {root.ID, root.Role, body, 200}} {
		w := call(MoneyAdjust, testRequest{method: "POST", target: "/api/wallet/adjustments", userID: sample.uid, role: sample.role, body: sample.body})
		if w.Code != sample.want {
			t.Fatalf("money request status=%d want=%d", w.Code, sample.want)
		}
	}
	if err := model.VerifyMoneyLedger(u.ID); err != nil {
		t.Fatal(err)
	}
}

func TestMoneyReconciliationReportsNanoProjectionDriftAsConflict(t *testing.T) {
	newTestDB(t)
	root := newUser(t, "root-reconcile", common.RoleRoot, 0)
	user := newUser(t, "reconcile-user", common.RoleUser, 1)
	if _, err := model.ReadMoneyAccount(user.ID); err != nil {
		t.Fatal(err)
	}
	if err := model.DB.Model(&model.User{}).Where("id = ?", user.ID).Update("quota_remainder_nano", 1).Error; err != nil {
		t.Fatal(err)
	}
	w := call(MoneyReconciliation, testRequest{method: "GET", target: "/api/wallet/reconciliation?user_id=" + itoa(user.ID), userID: root.ID, role: root.Role})
	expectStatus(t, w, 409)
}

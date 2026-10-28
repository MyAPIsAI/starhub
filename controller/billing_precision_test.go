package controller

import (
	"math"
	"testing"

	"starhub/common"
	"starhub/model"
)

func TestWalletAndLedgerExposeExactNanoStrings(t *testing.T) {
	newTestDB(t)
	user := newUser(t, "exact-wallet", common.RoleUser, 123)
	if err := model.DB.Create(&model.Wallet{UserID: user.ID, Balance: math.MaxInt64}).Error; err != nil {
		t.Fatal(err)
	}
	if err := model.DB.Create(&model.LedgerEntry{UserID: user.ID, Type: "adjustment", Amount: math.MinInt64, BalanceAfter: math.MaxInt64}).Error; err != nil {
		t.Fatal(err)
	}
	var wallet map[string]any
	decodeData(t, call(GetWallet, testRequest{target: "/api/wallet", userID: user.ID, role: user.Role}), &wallet)
	if wallet["balance_nano"] != "9223372036854775807" {
		t.Fatalf("exact wallet amount lost: %v", wallet)
	}
	if _, ok := wallet["balance"].(float64); !ok {
		t.Fatal("legacy numeric field removed")
	}
	var ledger []map[string]any
	decodeData(t, call(ListLedger, testRequest{target: "/api/ledger", userID: user.ID, role: user.Role}), &ledger)
	if len(ledger) != 1 || ledger[0]["amount_nano"] != "-9223372036854775808" || ledger[0]["balance_after_nano"] != "9223372036854775807" {
		t.Fatalf("exact ledger amounts lost: %v", ledger)
	}
	for _, field := range []string{"amount", "balance_after"} {
		if _, ok := ledger[0][field].(float64); !ok {
			t.Fatalf("legacy %s removed", field)
		}
	}
	after, err := model.GetUserByID(user.ID)
	if err != nil || after.Quota != 123 {
		t.Fatalf("read changed API quota: %+v %v", after, err)
	}
}

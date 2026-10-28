package main

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
)

func TestWalletExactStringsRetainOwnerScope(t *testing.T) {
	env := setupRelayApp(t)
	owner := &model.User{Username: "precise-owner", Role: common.RoleUser, Status: common.StatusEnabled, AffCode: common.GenerateAffCode()}
	if err := model.CreateUser(owner); err != nil {
		t.Fatal(err)
	}
	other := &model.User{Username: "precise-other", Role: common.RoleUser, Status: common.StatusEnabled, AffCode: common.GenerateAffCode()}
	if err := model.CreateUser(other); err != nil {
		t.Fatal(err)
	}
	for _, wallet := range []model.Wallet{{UserID: owner.ID, Balance: math.MaxInt64}, {UserID: other.ID, Balance: 7}} {
		if err := model.DB.Create(&wallet).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, entry := range []model.LedgerEntry{{UserID: owner.ID, Type: "adjustment", Amount: -9007199254740993, BalanceAfter: math.MaxInt64}, {UserID: other.ID, Type: "adjustment", Amount: 7, BalanceAfter: 7}} {
		if err := model.DB.Create(&entry).Error; err != nil {
			t.Fatal(err)
		}
	}
	session := middleware.Sessions.Create(owner.ID, time.Hour)
	t.Cleanup(func() { middleware.Sessions.Remove(session) })
	res := doRequest(env.base, "GET", "/api/wallet?user_id=1", session, nil)
	if res.Status != 200 {
		t.Fatalf("wallet: %d %s", res.Status, res.Body)
	}
	var wallet struct {
		Data struct {
			BalanceNano string `json:"balance_nano"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(res.Body), &wallet); err != nil || wallet.Data.BalanceNano != "9223372036854775807" {
		t.Fatalf("exact wallet: %s %v", res.Body, err)
	}
	res = doRequest(env.base, "GET", "/api/ledger?user_id=1", session, nil)
	var ledger struct {
		Data []struct {
			AmountNano       string `json:"amount_nano"`
			BalanceAfterNano string `json:"balance_after_nano"`
		} `json:"data"`
	}
	if res.Status != 200 {
		t.Fatalf("ledger: %d %s", res.Status, res.Body)
	}
	if err := json.Unmarshal([]byte(res.Body), &ledger); err != nil || len(ledger.Data) != 1 || ledger.Data[0].AmountNano != "-9007199254740993" || ledger.Data[0].BalanceAfterNano != "9223372036854775807" {
		t.Fatalf("scope or precision lost: %s %v", res.Body, err)
	}
}

package main

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"starhub/model"
)

// TestRetiredRedemptionCannotCredit verifies that even valid historical codes
// cannot change balances through concurrent requests to the retired API.
func TestRetiredRedemptionCannotCredit(t *testing.T) {
	env := setupRelayApp(t)
	code := &model.Redemption{Code: "RD-retired-concurrency", Quota: 500000, Status: 1}
	if err := model.InsertRedemption(code); err != nil {
		t.Fatal(err)
	}
	before, err := model.GetUserByID(env.rootID)
	if err != nil {
		t.Fatal(err)
	}
	const concurrency = 10
	var wg sync.WaitGroup
	var rejected atomic.Int64
	for range concurrency {
		wg.Go(func() {
			res := doRequest(env.base, "POST", "/api/user/redeem", env.session, map[string]any{"code": code.Code})
			if res.Status == 410 {
				rejected.Add(1)
			}
		})
	}
	wg.Wait()
	after, err := model.GetUserByID(env.rootID)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := model.GetRedemptionByCode(code.Code)
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Load() != concurrency || after.Quota != before.Quota || stored.Status != 1 || stored.UsedBy != 0 {
		t.Fatal("retired redemption credited funds or changed the historical code")
	}
}

// TestBILL006ConcurrentRechargeResolve verifies that concurrent resolve
// calls for the same recharge order only credit once.
func TestBILL006ConcurrentRechargeResolve(t *testing.T) {
	env := setupRelayApp(t)

	// 1. Create a recharge order.
	res := doRequest(env.base, "POST", "/api/recharge", env.session, map[string]any{
		"amount_usd": "10.00",
	})
	if res.Status != 200 {
		t.Fatalf("create recharge failed: %d %s", res.Status, res.Body)
	}
	var orderResp struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(res.Body), &orderResp); err != nil || orderResp.Data.ID == 0 {
		t.Fatalf("bad create order response: %s", res.Body)
	}
	orderID := orderResp.Data.ID

	// 2. Record wallet balance before.
	walletBefore, err := model.GetOrCreateWallet(env.rootID)
	if err != nil {
		t.Fatalf("GetOrCreateWallet before failed: %v", err)
	}

	// 3. Fire N concurrent resolve(success) calls.
	const concurrency = 10
	var wg sync.WaitGroup
	var successCount int64
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			r := doRequest(env.base, "POST", fmt.Sprintf("/api/recharge/%d/resolve", orderID), env.session, map[string]any{
				"outcome": "success",
			})
			if r.Status == 200 {
				atomic.AddInt64(&successCount, 1)
			}
		}()
	}
	wg.Wait()

	// 4. Wallet must have increased by exactly 10 USD (in nano-USD).
	walletAfter, err := model.GetOrCreateWallet(env.rootID)
	if err != nil {
		t.Fatalf("GetOrCreateWallet after failed: %v", err)
	}
	expectedDelta := int64(10 * 1e9) // 10 USD in nano-USD
	actualDelta := walletAfter.Balance - walletBefore.Balance
	if actualDelta != expectedDelta {
		t.Fatalf("wallet delta = %d, want %d (before=%d, after=%d)",
			actualDelta, expectedDelta, walletBefore.Balance, walletAfter.Balance)
	}

	// 5. Ledger must have exactly one recharge entry for this order.
	orders, err := model.ListRechargeOrders(env.rootID)
	if err != nil {
		t.Fatalf("ListRechargeOrders failed: %v", err)
	}
	var orderNumber string
	for _, o := range orders {
		if o.ID == orderID {
			orderNumber = o.OrderNumber
			break
		}
	}
	if orderNumber == "" {
		t.Fatalf("could not find order number for order ID %d", orderID)
	}
	entries, err := model.ListLedger(env.rootID)
	if err != nil {
		t.Fatalf("ListLedger failed: %v", err)
	}
	rechargeCount := 0
	for _, e := range entries {
		if e.OrderID == orderNumber && e.Type == "recharge" {
			rechargeCount++
		}
	}
	if rechargeCount != 1 {
		t.Fatalf("expected 1 recharge ledger entry, got %d", rechargeCount)
	}

	t.Logf("BILL-006 recharge: %d concurrent resolve, wallet delta=%d nano-USD, 1 ledger entry",
		concurrency, actualDelta)
}

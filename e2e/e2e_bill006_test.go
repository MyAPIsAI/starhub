package main

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"starhub/model"
)

// TestBILL006ConcurrentRedemption verifies that concurrent attempts to
// redeem the same code only credit the user once. This is BILL-006 from
// the P21 test strategy: "同兑换码并发执行 → 一次资金增量".
func TestBILL006ConcurrentRedemption(t *testing.T) {
	env := setupRelayApp(t)

	// 1. Admin creates a redemption code with 500000 quota ($1).
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
	if err := json.Unmarshal([]byte(res.Body), &rdResp); err != nil || len(rdResp.Data.Codes) != 1 {
		t.Fatalf("expected 1 code, got: %s", res.Body)
	}
	code := rdResp.Data.Codes[0]

	// 2. Record the user's quota before redemption.
	before, err := model.GetUserByID(env.rootID)
	if err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}

	// 3. Fire N concurrent redeem requests for the same code.
	const concurrency = 10
	var wg sync.WaitGroup
	var successCount int64
	var failCount int64
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			r := doRequest(env.base, "POST", "/api/user/redeem", env.session, map[string]any{
				"code": code,
			})
			if r.Status == 200 {
				atomic.AddInt64(&successCount, 1)
			} else {
				atomic.AddInt64(&failCount, 1)
			}
		}()
	}
	wg.Wait()

	// 4. Exactly one request must have succeeded; all others must fail.
	if successCount != 1 {
		t.Fatalf("expected exactly 1 successful redeem, got %d (failures: %d)", successCount, failCount)
	}
	if failCount != concurrency-1 {
		t.Fatalf("expected %d failures, got %d", concurrency-1, failCount)
	}

	// 5. Quota increased by exactly 500000 — no double crediting.
	after, err := model.GetUserByID(env.rootID)
	if err != nil {
		t.Fatalf("GetUserByID after failed: %v", err)
	}
	expected := before.Quota + 500000
	if after.Quota != expected {
		t.Fatalf("quota after concurrent redeem = %d, want %d (before=%d, delta should be exactly 500000)",
			after.Quota, expected, before.Quota)
	}

	// 6. The redemption code must be in "used" status (2).
	rd, err := model.GetRedemptionByCode(code)
	if err != nil {
		t.Fatalf("GetRedemptionByCode failed: %v", err)
	}
	if rd.Status != 2 {
		t.Fatalf("redemption status = %d, want 2 (used)", rd.Status)
	}
	if rd.UsedBy != env.rootID {
		t.Fatalf("redemption used_by = %d, want %d", rd.UsedBy, env.rootID)
	}

	t.Logf("BILL-006 redemption: %d concurrent attempts, 1 success, %d failures, quota delta=500000",
		concurrency, failCount)
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

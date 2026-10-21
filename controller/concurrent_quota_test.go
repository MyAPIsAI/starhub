package controller

import (
	"fmt"
	"net/http"
	"testing"

	"starhub/common"
	"starhub/model"
)

// TestConcurrentExpectedQuotaConflict verifies that when two concurrent
// admin updates target the same user's quota, the second update with a
// stale expected_quota is rejected with 409 Conflict. The first update's
// changes must be fully applied; the second must not partially overwrite
// any field. This is section 6.3 of the P21 test strategy.
func TestConcurrentExpectedQuotaConflict(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 1000)
	id := itoa(u.ID)

	// 1. First update: quota 1000 -> 800, status disabled.
	//    Uses the correct expected_quota (1000).
	rr := call(UpdateUser, testRequest{
		method: http.MethodPut,
		target: "/api/user",
		body:   fmt.Sprintf(`{"id":%s,"quota":800,"expected_quota":1000,"status":%d}`, id, common.StatusDisabled),
		userID: 1,
		role:   common.RoleAdmin,
	})
	expectStatus(t, rr, http.StatusOK)

	// Verify first update applied.
	got, _ := model.GetUserByID(u.ID)
	if got.Quota != 800 {
		t.Fatalf("after first update: quota = %d, want 800", got.Quota)
	}
	if got.Status != common.StatusDisabled {
		t.Fatalf("after first update: status = %d, want %d", got.Status, common.StatusDisabled)
	}

	// 2. Second update: quota 800 -> 600, but uses STALE expected_quota=1000.
	//    Must be rejected with 409 Conflict.
	rr = call(UpdateUser, testRequest{
		method: http.MethodPut,
		target: "/api/user",
		body:   fmt.Sprintf(`{"id":%s,"quota":600,"expected_quota":1000}`, id),
		userID: 1,
		role:   common.RoleAdmin,
	})
	expectError(t, rr, http.StatusConflict, "balance changed")

	// 3. Verify the second update did NOT change anything.
	got2, _ := model.GetUserByID(u.ID)
	if got2.Quota != 800 {
		t.Fatalf("after stale update: quota = %d, want 800 (must not change)", got2.Quota)
	}
	if got2.Status != common.StatusDisabled {
		t.Fatalf("after stale update: status = %d, want %d (must not change)", got2.Status, common.StatusDisabled)
	}

	// 4. Third update with the CORRECT expected_quota (800) should succeed.
	rr = call(UpdateUser, testRequest{
		method: http.MethodPut,
		target: "/api/user",
		body:   fmt.Sprintf(`{"id":%s,"quota":600,"expected_quota":800}`, id),
		userID: 1,
		role:   common.RoleAdmin,
	})
	expectStatus(t, rr, http.StatusOK)

	got3, _ := model.GetUserByID(u.ID)
	if got3.Quota != 600 {
		t.Fatalf("after correct update: quota = %d, want 600", got3.Quota)
	}

	// 5. Verify that a multi-field update with stale expected_quota does
	//    not partially apply any field (no partial write).
	rr = call(UpdateUser, testRequest{
		method: http.MethodPut,
		target: "/api/user",
		body:   fmt.Sprintf(`{"id":%s,"quota":999,"expected_quota":800,"status":%d,"group":%q}`, id, common.StatusEnabled, common.DefaultGroup),
		userID: 1,
		role:   common.RoleAdmin,
	})
	expectError(t, rr, http.StatusConflict, "balance changed")

	got4, _ := model.GetUserByID(u.ID)
	if got4.Quota != 600 {
		t.Fatalf("after stale multi-field update: quota = %d, want 600 (no partial write)", got4.Quota)
	}
	if got4.Status != common.StatusDisabled {
		t.Fatalf("after stale multi-field update: status = %d, want %d (no partial write)", got4.Status, common.StatusDisabled)
	}

	t.Logf("6.3 concurrent expected_quota: stale -> 409 conflict, no partial write; correct -> 200 ok")
}

// TestRechargeOrderStateMachine verifies that recharge order state
// transitions follow the defined rules: pending -> processing -> paid,
// and terminal states (paid/failed/cancelled/expired) are idempotent.
// This is section 6.3 of the P21 test strategy.
func TestRechargeOrderStateMachine(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 1000)

	// 1. Create a recharge order via the handler.
	rr := call(CreateRechargeOrder, testRequest{
		method: http.MethodPost,
		target: "/api/recharge",
		body:   `{"amount_usd":"10.00"}`,
		userID: u.ID,
		role:   common.RoleUser,
	})
	var order struct {
		ID     int64  `json:"id"`
		Status string `json:"status"`
	}
	decodeData(t, rr, &order)
	if order.Status != "pending" {
		t.Fatalf("initial status = %q, want pending", order.Status)
	}

	// 2. Begin payment: pending -> processing.
	rr = call(BeginRechargePayment, testRequest{
		method: http.MethodPost,
		target: fmt.Sprintf("/api/recharge/%d/begin", order.ID),
		path:   map[string]string{"id": fmt.Sprintf("%d", order.ID)},
		userID: u.ID,
		role:   common.RoleUser,
	})
	expectStatus(t, rr, http.StatusOK)

	// 3. Resolve as success: processing -> paid.
	rr = call(ResolveRechargeOrder, testRequest{
		method: http.MethodPost,
		target: fmt.Sprintf("/api/recharge/%d/resolve", order.ID),
		body:   `{"outcome":"success"}`,
		path:   map[string]string{"id": fmt.Sprintf("%d", order.ID)},
		userID: u.ID,
		role:   common.RoleUser,
	})
	expectStatus(t, rr, http.StatusOK)

	// 4. Verify wallet was credited.
	wallet, err := model.GetOrCreateWallet(u.ID)
	if err != nil {
		t.Fatalf("GetOrCreateWallet failed: %v", err)
	}
	if wallet.Balance != int64(10*1e9) { // 10 USD in nano
		t.Fatalf("wallet balance = %d, want %d (10 USD in nano)", wallet.Balance, int64(10*1e9))
	}

	// 5. Resolve again: paid is terminal, must be idempotent (no double credit).
	walletBefore := wallet.Balance
	rr = call(ResolveRechargeOrder, testRequest{
		method: http.MethodPost,
		target: fmt.Sprintf("/api/recharge/%d/resolve", order.ID),
		body:   `{"outcome":"success"}`,
		path:   map[string]string{"id": fmt.Sprintf("%d", order.ID)},
		userID: u.ID,
		role:   common.RoleUser,
	})
	expectStatus(t, rr, http.StatusOK)

	walletAfter, _ := model.GetOrCreateWallet(u.ID)
	if walletAfter.Balance != walletBefore {
		t.Fatalf("double resolve: wallet = %d, want %d (idempotent)", walletAfter.Balance, walletBefore)
	}

	// 6. Create another order and resolve as failed: pending -> failed (no credit).
	rr = call(CreateRechargeOrder, testRequest{
		method: http.MethodPost,
		target: "/api/recharge",
		body:   `{"amount_usd":"20.00"}`,
		userID: u.ID,
		role:   common.RoleUser,
	})
	var order2 struct {
		ID int64 `json:"id"`
	}
	decodeData(t, rr, &order2)

	walletBeforeFail := walletAfter.Balance
	rr = call(ResolveRechargeOrder, testRequest{
		method: http.MethodPost,
		target: fmt.Sprintf("/api/recharge/%d/resolve", order2.ID),
		body:   `{"outcome":"failed"}`,
		path:   map[string]string{"id": fmt.Sprintf("%d", order2.ID)},
		userID: u.ID,
		role:   common.RoleUser,
	})
	expectStatus(t, rr, http.StatusOK)

	walletAfterFail, _ := model.GetOrCreateWallet(u.ID)
	if walletAfterFail.Balance != walletBeforeFail {
		t.Fatalf("failed resolve: wallet = %d, want %d (no credit on failure)", walletAfterFail.Balance, walletBeforeFail)
	}

	// 7. Cancel a terminal order: failed is terminal, must be idempotent.
	rr = call(ResolveRechargeOrder, testRequest{
		method: http.MethodPost,
		target: fmt.Sprintf("/api/recharge/%d/resolve", order2.ID),
		body:   `{"outcome":"cancelled"}`,
		path:   map[string]string{"id": fmt.Sprintf("%d", order2.ID)},
		userID: u.ID,
		role:   common.RoleUser,
	})
	expectStatus(t, rr, http.StatusOK) // idempotent: returns the existing terminal state

	t.Logf("6.3 recharge state machine: pending->processing->paid (credited), double resolve idempotent, failed (no credit), terminal idempotent")
}

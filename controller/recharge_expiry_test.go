package controller

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"starhub/model"
)

// TestRechargeOrderExpiryBoundary verifies recharge order expiry at the
// exact boundary: before expiry, at expiry (now == expiresAt), and after
// expiry. This addresses REVIEW-08: "recharge state machine missing
// complete expiry boundary tests".
func TestRechargeOrderExpiryBoundary(t *testing.T) {
	t.Run("resolve before expiry succeeds", func(t *testing.T) {
		newTestDB(t)
		u := newUser(t, "alice", 1, 1000)

		// Create order via handler with valid amount.
		rr := call(CreateRechargeOrder, testRequest{
			method: http.MethodPost,
			target: "/api/recharge",
			body:   `{"amount_usd":"10.00"}`,
			userID: u.ID,
			role:   1,
		})
		var order map[string]any
		decodeData(t, rr, &order)
		orderID := int64(order["id"].(float64))

		// Resolve immediately — should succeed (not expired).
		rr = call(ResolveRechargeOrder, testRequest{
			method: http.MethodPost,
			target: fmt.Sprintf("/api/recharge/%d/resolve", orderID),
			body:   `{"outcome":"success"}`,
			userID: u.ID,
			role:   1,
			path:   map[string]string{"id": fmt.Sprintf("%d", orderID)},
		})
		var resolved map[string]any
		decodeData(t, rr, &resolved)
		if resolved["status"] != "paid" {
			t.Fatalf("status = %v, want paid (resolved before expiry)", resolved["status"])
		}

		// Wallet should be credited.
		w, _ := model.GetOrCreateWallet(u.ID)
		if w.Balance != int64(10*1e9) {
			t.Fatalf("wallet balance = %d, want %d", w.Balance, int64(10*1e9))
		}
	})

	t.Run("resolve at exact expiry time succeeds (now == expiresAt is NOT expired)", func(t *testing.T) {
		newTestDB(t)
		u := newUser(t, "bob", 1, 1000)

		// Create order via handler, then update ExpiresAt to now via DB.
		rr := call(CreateRechargeOrder, testRequest{
			method: http.MethodPost,
			target: "/api/recharge",
			body:   `{"amount_usd":"10.00"}`,
			userID: u.ID,
			role:   1,
		})
		var order map[string]any
		decodeData(t, rr, &order)
		orderID := int64(order["id"].(float64))

		// Set ExpiresAt to now via DB (CreateRechargeOrder sets it to now+30min).
		model.DB.Model(&model.RechargeOrder{}).Where("id = ?", orderID).
			Update("expires_at", time.Now().Unix())

		// Resolve — code uses `now > o.ExpiresAt` (strictly greater),
		// so now == ExpiresAt should NOT trigger expiry.
		resolved, err := model.ResolveRechargeOrder(orderID, "success")
		if err != nil {
			t.Fatalf("ResolveRechargeOrder at boundary failed: %v", err)
		}
		if resolved.Status != model.RechargePaid {
			t.Fatalf("status = %q, want paid (now == expiresAt is not expired)", resolved.Status)
		}

		w, _ := model.GetOrCreateWallet(u.ID)
		if w.Balance != int64(10*1e9) {
			t.Fatalf("wallet balance = %d, want %d", w.Balance, int64(10*1e9))
		}
	})

	t.Run("resolve after expiry marks as expired and does not credit", func(t *testing.T) {
		newTestDB(t)
		u := newUser(t, "carol", 1, 1000)

		// Create order that already expired (ExpiresAt = now - 60).
		o := &model.RechargeOrder{
			OrderNumber: "ord_test_expired",
			UserID:      u.ID,
			Amount:      20 * 1e9,
			Status:      model.RechargePending,
			ExpiresAt:   time.Now().Unix() - 60,
		}
		if err := model.CreateRechargeOrder(o); err != nil {
			t.Fatalf("CreateRechargeOrder failed: %v", err)
		}

		// Resolve — should be marked as expired, not credited.
		resolved, err := model.ResolveRechargeOrder(o.ID, "success")
		if err != nil {
			t.Fatalf("ResolveRechargeOrder failed: %v", err)
		}
		if resolved.Status != model.RechargeExpired {
			t.Fatalf("status = %q, want expired (resolved after expiry)", resolved.Status)
		}
		if resolved.FailureReason != "payment_expired" {
			t.Fatalf("failure_reason = %q, want payment_expired", resolved.FailureReason)
		}

		// Wallet should NOT be credited.
		w, _ := model.GetOrCreateWallet(u.ID)
		if w.Balance != 0 {
			t.Fatalf("wallet balance = %d, want 0 (expired order should not credit)", w.Balance)
		}

		// No ledger entry.
		entries, _ := model.ListLedger(u.ID)
		if len(entries) != 0 {
			t.Fatalf("expected 0 ledger entries, got %d", len(entries))
		}
	})

	t.Run("expired order is idempotent on repeated resolve", func(t *testing.T) {
		newTestDB(t)
		u := newUser(t, "dave", 1, 1000)

		o := &model.RechargeOrder{
			OrderNumber: "ord_test_idempotent",
			UserID:      u.ID,
			Amount:      15 * 1e9,
			Status:      model.RechargePending,
			ExpiresAt:   time.Now().Unix() - 60,
		}
		if err := model.CreateRechargeOrder(o); err != nil {
			t.Fatalf("CreateRechargeOrder failed: %v", err)
		}

		// First resolve → expired.
		r1, _ := model.ResolveRechargeOrder(o.ID, "success")
		if r1.Status != model.RechargeExpired {
			t.Fatalf("first resolve: status = %q, want expired", r1.Status)
		}

		// Second resolve → still expired (idempotent, no state change).
		r2, _ := model.ResolveRechargeOrder(o.ID, "success")
		if r2.Status != model.RechargeExpired {
			t.Fatalf("second resolve: status = %q, want expired (idempotent)", r2.Status)
		}

		// Wallet still 0.
		w, _ := model.GetOrCreateWallet(u.ID)
		if w.Balance != 0 {
			t.Fatalf("wallet = %d, want 0", w.Balance)
		}
	})
}

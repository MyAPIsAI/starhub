package controller

import (
	"net/http"
	"testing"

	"starhub/common"
)

func TestGetWalletCreatesLazy(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 1000)

	rr := call(GetWallet, testRequest{
		method: http.MethodGet,
		target: "/api/wallet",
		userID: u.ID,
		role:   common.RoleUser,
	})
	var got map[string]any
	decodeData(t, rr, &got)
	if got["balance"].(float64) != 0 {
		t.Fatalf("new wallet balance = %v, want 0", got["balance"])
	}
	if got["user_id"].(float64) != float64(u.ID) {
		t.Fatalf("user_id = %v, want %d", got["user_id"], u.ID)
	}
}

func TestCreateRechargeOrderValidation(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 1000)

	cases := []struct {
		name   string
		amount string
	}{
		{"below minimum", "5.00"},
		{"above maximum", "10001.00"},
		{"three decimals", "100.001"},
		{"empty", ""},
		{"non-numeric", "abc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := call(CreateRechargeOrder, testRequest{
				method: http.MethodPost,
				target: "/api/recharge",
				body:   `{"amount_usd":"` + tc.amount + `"}`,
				userID: u.ID,
				role:   common.RoleUser,
			})
			expectError(t, rr, http.StatusBadRequest, "amount_usd")
		})
	}
}

func TestBillingFundingClaimsAndTrailingDocumentsAreRejected(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "funding-owner", common.RoleUser, 0)
	for _, body := range []string{`{"amount_usd":"10.00","funding_source":"verified_provider"}`, `{"amount_usd":"10.00","api_quota_eligible":true}`, `{"amount_usd":"10.00"} {}`, `{"amount_usd":"10.00","paid_at":123}`, `{"amount_usd":"10.00","provider_transaction_id":"invented"}`} {
		expectStatus(t, call(CreateRechargeOrder, testRequest{method: "POST", target: "/api/recharge", userID: u.ID, role: common.RoleUser, body: body}), http.StatusBadRequest)
	}
}

func TestCreateRechargeOrderSuccess(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 1000)

	rr := call(CreateRechargeOrder, testRequest{
		method: http.MethodPost,
		target: "/api/recharge",
		body:   `{"amount_usd":"100.00"}`,
		userID: u.ID,
		role:   common.RoleUser,
	})
	var got map[string]any
	decodeData(t, rr, &got)
	if got["status"] != "pending" {
		t.Fatalf("status = %v, want pending", got["status"])
	}
	if got["amount_nano"].(float64) != 100_000_000_000 {
		t.Fatalf("amount_nano = %v, want 100000000000", got["amount_nano"])
	}
	if got["order_number"] == "" {
		t.Fatal("order_number should not be empty")
	}
}

func TestResolveRechargeOrderSuccessCredits(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 1000)

	// Create order.
	rr := call(CreateRechargeOrder, testRequest{
		method: http.MethodPost,
		target: "/api/recharge",
		body:   `{"amount_usd":"50.00"}`,
		userID: u.ID,
		role:   common.RoleUser,
	})
	var order map[string]any
	decodeData(t, rr, &order)
	orderID := int64(order["id"].(float64))

	// Resolve as success.
	rr = call(ResolveRechargeOrder, testRequest{
		method: http.MethodPost,
		target: "/api/recharge/" + itoa(orderID) + "/resolve",
		body:   `{"outcome":"success"}`,
		userID: u.ID,
		role:   common.RoleUser,
		path:   map[string]string{"id": itoa(orderID)},
	})
	var resolved map[string]any
	decodeData(t, rr, &resolved)
	if resolved["status"] != "paid" {
		t.Fatalf("status = %v, want paid", resolved["status"])
	}

	// Wallet should now have 50 USD = 50_000_000_000 nano.
	rr = call(GetWallet, testRequest{
		method: http.MethodGet,
		target: "/api/wallet",
		userID: u.ID,
		role:   common.RoleUser,
	})
	var wallet map[string]any
	decodeData(t, rr, &wallet)
	if wallet["balance"].(float64) != 50_000_000_000 {
		t.Fatalf("balance = %v, want 50000000000", wallet["balance"])
	}

	// Ledger should have one recharge entry.
	rr = call(ListLedger, testRequest{
		method: http.MethodGet,
		target: "/api/ledger",
		userID: u.ID,
		role:   common.RoleUser,
	})
	var entries []map[string]any
	decodeData(t, rr, &entries)
	if len(entries) != 1 {
		t.Fatalf("ledger entries = %d, want 1", len(entries))
	}
	if entries[0]["type"] != "recharge" {
		t.Fatalf("type = %v, want recharge", entries[0]["type"])
	}
	if entries[0]["amount"].(float64) != 50_000_000_000 {
		t.Fatalf("amount = %v, want 50000000000", entries[0]["amount"])
	}
}

func TestResolveRechargeOrderDoubleResolveNoDoubleCredit(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 1000)

	rr := call(CreateRechargeOrder, testRequest{
		method: http.MethodPost,
		target: "/api/recharge",
		body:   `{"amount_usd":"100.00"}`,
		userID: u.ID,
		role:   common.RoleUser,
	})
	var order map[string]any
	decodeData(t, rr, &order)
	orderID := int64(order["id"].(float64))

	// First resolve — success.
	call(ResolveRechargeOrder, testRequest{
		method: http.MethodPost,
		target: "/api/recharge/" + itoa(orderID) + "/resolve",
		body:   `{"outcome":"success"}`,
		userID: u.ID,
		role:   common.RoleUser,
		path:   map[string]string{"id": itoa(orderID)},
	})
	// Second resolve — should be idempotent.
	call(ResolveRechargeOrder, testRequest{
		method: http.MethodPost,
		target: "/api/recharge/" + itoa(orderID) + "/resolve",
		body:   `{"outcome":"success"}`,
		userID: u.ID,
		role:   common.RoleUser,
		path:   map[string]string{"id": itoa(orderID)},
	})

	// Balance must be 100, not 200.
	rr = call(GetWallet, testRequest{
		method: http.MethodGet,
		target: "/api/wallet",
		userID: u.ID,
		role:   common.RoleUser,
	})
	var wallet map[string]any
	decodeData(t, rr, &wallet)
	if wallet["balance"].(float64) != 100_000_000_000 {
		t.Fatalf("balance = %v, want 100000000000 (no double credit)", wallet["balance"])
	}
}

func TestResolveRechargeOrderFailedDoesNotCredit(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 1000)

	rr := call(CreateRechargeOrder, testRequest{
		method: http.MethodPost,
		target: "/api/recharge",
		body:   `{"amount_usd":"100.00"}`,
		userID: u.ID,
		role:   common.RoleUser,
	})
	var order map[string]any
	decodeData(t, rr, &order)
	orderID := int64(order["id"].(float64))

	call(ResolveRechargeOrder, testRequest{
		method: http.MethodPost,
		target: "/api/recharge/" + itoa(orderID) + "/resolve",
		body:   `{"outcome":"failed"}`,
		userID: u.ID,
		role:   common.RoleUser,
		path:   map[string]string{"id": itoa(orderID)},
	})

	rr = call(GetWallet, testRequest{
		method: http.MethodGet,
		target: "/api/wallet",
		userID: u.ID,
		role:   common.RoleUser,
	})
	var wallet map[string]any
	decodeData(t, rr, &wallet)
	if wallet["balance"].(float64) != 0 {
		t.Fatalf("balance = %v, want 0 (failed recharge must not credit)", wallet["balance"])
	}
}

func TestGetRechargeOrderForbiddenForOtherUser(t *testing.T) {
	newTestDB(t)
	alice := newUser(t, "alice", common.RoleUser, 1000)
	bob := newUser(t, "bob", common.RoleUser, 1000)

	rr := call(CreateRechargeOrder, testRequest{
		method: http.MethodPost,
		target: "/api/recharge",
		body:   `{"amount_usd":"100.00"}`,
		userID: alice.ID,
		role:   common.RoleUser,
	})
	var order map[string]any
	decodeData(t, rr, &order)
	orderID := int64(order["id"].(float64))

	// Bob cannot see Alice's order.
	rr = call(GetRechargeOrder, testRequest{
		method: http.MethodGet,
		target: "/api/recharge/" + itoa(orderID),
		userID: bob.ID,
		role:   common.RoleUser,
		path:   map[string]string{"id": itoa(orderID)},
	})
	expectError(t, rr, http.StatusForbidden, "another user")
}

func TestListRechargeOrdersUserSeesOwnOnly(t *testing.T) {
	newTestDB(t)
	alice := newUser(t, "alice", common.RoleUser, 1000)
	bob := newUser(t, "bob", common.RoleUser, 1000)

	// Alice creates an order.
	call(CreateRechargeOrder, testRequest{
		method: http.MethodPost,
		target: "/api/recharge",
		body:   `{"amount_usd":"100.00"}`,
		userID: alice.ID,
		role:   common.RoleUser,
	})

	// Bob lists orders — should see none.
	rr := call(ListRechargeOrders, testRequest{
		method: http.MethodGet,
		target: "/api/recharge",
		userID: bob.ID,
		role:   common.RoleUser,
	})
	var orders []map[string]any
	decodeData(t, rr, &orders)
	if len(orders) != 0 {
		t.Fatalf("bob sees %d orders, want 0", len(orders))
	}

	// Alice lists orders — should see 1.
	rr = call(ListRechargeOrders, testRequest{
		method: http.MethodGet,
		target: "/api/recharge",
		userID: alice.ID,
		role:   common.RoleUser,
	})
	decodeData(t, rr, &orders)
	if len(orders) != 1 {
		t.Fatalf("alice sees %d orders, want 1", len(orders))
	}
}

func TestBeginRechargePaymentTransitionsToProcessing(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 1000)

	rr := call(CreateRechargeOrder, testRequest{
		method: http.MethodPost,
		target: "/api/recharge",
		body:   `{"amount_usd":"100.00"}`,
		userID: u.ID,
		role:   common.RoleUser,
	})
	var order map[string]any
	decodeData(t, rr, &order)
	orderID := int64(order["id"].(float64))

	rr = call(BeginRechargePayment, testRequest{
		method: http.MethodPost,
		target: "/api/recharge/" + itoa(orderID) + "/begin",
		userID: u.ID,
		role:   common.RoleUser,
		path:   map[string]string{"id": itoa(orderID)},
	})
	var got map[string]any
	decodeData(t, rr, &got)
	if got["status"] != "processing" {
		t.Fatalf("status = %v, want processing", got["status"])
	}
}

func TestResolveRechargeOrderInvalidOutcome(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 1000)

	rr := call(CreateRechargeOrder, testRequest{
		method: http.MethodPost,
		target: "/api/recharge",
		body:   `{"amount_usd":"100.00"}`,
		userID: u.ID,
		role:   common.RoleUser,
	})
	var order map[string]any
	decodeData(t, rr, &order)
	orderID := int64(order["id"].(float64))

	rr = call(ResolveRechargeOrder, testRequest{
		method: http.MethodPost,
		target: "/api/recharge/" + itoa(orderID) + "/resolve",
		body:   `{"outcome":"invalid"}`,
		userID: u.ID,
		role:   common.RoleUser,
		path:   map[string]string{"id": itoa(orderID)},
	})
	expectError(t, rr, http.StatusBadRequest, "outcome")
}

func TestAdminCanListAllRechargeOrders(t *testing.T) {
	newTestDB(t)
	alice := newUser(t, "alice", common.RoleUser, 1000)
	admin := newUser(t, "admin", common.RoleAdmin, 1000)

	call(CreateRechargeOrder, testRequest{
		method: http.MethodPost,
		target: "/api/recharge",
		body:   `{"amount_usd":"100.00"}`,
		userID: alice.ID,
		role:   common.RoleUser,
	})

	// Admin sees all orders.
	rr := call(ListRechargeOrders, testRequest{
		method: http.MethodGet,
		target: "/api/recharge",
		userID: admin.ID,
		role:   common.RoleAdmin,
	})
	var orders []map[string]any
	decodeData(t, rr, &orders)
	if len(orders) != 1 {
		t.Fatalf("admin sees %d orders, want 1", len(orders))
	}
}

func TestParseRechargeAmountEdgeCases(t *testing.T) {
	cases := []struct {
		input string
		want  int64
		ok    bool
	}{
		{"10.00", 10_000_000_000, true},
		{"10000.00", 10_000_000_000_000, true},
		{"100", 100_000_000_000, true},
		{"9.99", 0, false},
		{"10000.01", 0, false},
		{"100.5", 100_500_000_000, true}, // one decimal is valid
	}
	for _, tc := range cases {
		got, ok := parseRechargeAmount(tc.input)
		if ok != tc.ok {
			t.Errorf("parseRechargeAmount(%q) ok = %v, want %v", tc.input, ok, tc.ok)
			continue
		}
		if ok && got != tc.want {
			t.Errorf("parseRechargeAmount(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestMultipleRechargesAccumulate(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 1000)

	for _, amt := range []string{"10.00", "20.00", "30.00"} {
		rr := call(CreateRechargeOrder, testRequest{
			method: http.MethodPost,
			target: "/api/recharge",
			body:   `{"amount_usd":"` + amt + `"}`,
			userID: u.ID,
			role:   common.RoleUser,
		})
		var order map[string]any
		decodeData(t, rr, &order)
		orderID := int64(order["id"].(float64))
		call(ResolveRechargeOrder, testRequest{
			method: http.MethodPost,
			target: "/api/recharge/" + itoa(orderID) + "/resolve",
			body:   `{"outcome":"success"}`,
			userID: u.ID,
			role:   common.RoleUser,
			path:   map[string]string{"id": itoa(orderID)},
		})
	}

	rr := call(GetWallet, testRequest{
		method: http.MethodGet,
		target: "/api/wallet",
		userID: u.ID,
		role:   common.RoleUser,
	})
	var wallet map[string]any
	decodeData(t, rr, &wallet)
	// 10 + 20 + 30 = 60 USD = 60_000_000_000 nano
	if wallet["balance"].(float64) != 60_000_000_000 {
		t.Fatalf("balance = %v, want 60000000000", wallet["balance"])
	}
}

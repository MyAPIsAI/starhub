package main

import (
	"fmt"
	"testing"

	"starhub/model"
)

func TestOperatingDashboardVerifiedStaffAccess(t *testing.T) {
	handler, root := staffFixture(t)
	root.call("GET", "/api/admin/dashboard", nil, 401)
	root.enroll("root", "Root-temporary-123")
	root.call("GET", "/api/admin/dashboard?range=today", nil, 200)
	for _, role := range []int{5, 10, 20} {
		name := fmt.Sprintf("dashboard-role-%d", role)
		root.call("POST", "/api/admin/staff", map[string]any{"username": name, "temporary_password": "Dashboard-temporary-123", "role": role, "reason": "Dashboard role fixture"}, 200)
		client := &staffHTTP{t: t, handler: handler, ip: fmt.Sprintf("127.0.0.1:%d", 19000+role)}
		client.call("POST", "/api/admin/auth/login", map[string]string{"username": name, "password": "Dashboard-temporary-123"}, 200)
		client.call("GET", "/api/admin/dashboard", nil, 403)
		client.enroll(name, "Dashboard-temporary-123")
		want := 200
		if role == 20 {
			want = 403
		}
		client.call("GET", "/api/admin/dashboard", nil, want)
	}
	customer := &model.User{Username: "dashboard-owner", Role: 1, Status: 1, AffCode: "dash-owner"}
	if err := model.CreateUser(customer); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		root.call("POST", fmt.Sprintf("/api/admin/customers/%d/adjustments", customer.ID), map[string]any{"amount_usd": "100", "kind": "offline_payment", "reason": "Synthetic receipt", "operation_id": fmt.Sprintf("dashboard-receipt-%d", i)}, 200)
	}
	result := root.call("GET", "/api/admin/dashboard?range=today", nil, 200)
	totals := result["totals"].(map[string]any)
	if totals["recharge_nano"] != "300000000000" || totals["paying_accounts"] != float64(1) {
		t.Fatalf("actual receipt flow missing: %+v", totals)
	}
}

package main

import (
	"fmt"
	"starhub/config"
	"testing"
	"time"
)

func TestReconciliationStatementPermissions(t *testing.T) {
	handler, root := staffFixture(t)
	root.enroll("root", "Root-temporary-123")
	loc, _ := time.LoadLocation(config.RegionTimezone())
	day := time.Now().In(loc).AddDate(0, 0, -1).Format("2006-01-02")
	input := map[string]any{"provider": "synthetic-bank", "day": day, "source": "empty.csv", "csv": "provider_order_id,platform_order_id,user_id,amount_cents,currency,paid_at\n", "expected_previous_id": 0}
	root.call("POST", "/api/admin/reconciliation/statements", input, 200)
	for _, role := range []int{5, 10, 20} {
		name := fmt.Sprintf("reconciliation-staff-%d", role)
		root.call("POST", "/api/admin/staff", map[string]any{"username": name, "temporary_password": "Synthetic-temporary-123", "role": role, "reason": "Acceptance fixture"}, 200)
		actor := &staffHTTP{t: t, handler: handler, ip: fmt.Sprintf("127.0.0.1:%d", 19000+role)}
		actor.enroll(name, "Synthetic-temporary-123")
		want := 403
		if role == 5 {
			want = 200
		}
		actor.call("GET", "/api/admin/reconciliation/statements", nil, want)
		actor.call("POST", "/api/admin/reconciliation/statements", input, want)
		actor.call("GET", "/api/admin/reconciliation", nil, want)
	}
	data := root.call("GET", "/api/admin/reconciliation/statements", nil, 200)
	if len(data["items"].([]any)) != 1 {
		t.Fatal("repeated imports were not idempotent")
	}
}

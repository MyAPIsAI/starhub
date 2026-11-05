package main

import (
	"fmt"
	"starhub/model"
	"testing"
)

func TestFinanceRoleAssignmentCannotPromoteCustomerAccounts(t *testing.T) {
	env := setupRelayApp(t)
	id, session := exportTestUser(t, env.base, "finance-target")
	path := fmt.Sprintf("/api/user/%d/finance-role", id)
	body := map[string]any{"enabled": true, "expected_role": 1}
	for _, tc := range []struct {
		session string
		want    int
	}{{session, 403}, {env.session, 409}} {
		res := doRequest(env.base, "PUT", path, tc.session, body)
		if res.Status != tc.want {
			t.Fatalf("promote customer: %d %s", res.Status, res.Body)
		}
	}
	fresh, _ := model.GetUserByID(id)
	if fresh.AccountKind != "customer" || fresh.Role != 1 {
		t.Fatal("customer became staff")
	}
}

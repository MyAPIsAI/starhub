package controller

import (
	"starhub/common"
	"starhub/model"
	"testing"
)

func TestFinanceRoleAssignmentRequiresSeparateStaffIdentity(t *testing.T) {
	newTestDB(t)
	user := newUser(t, "finance-target", common.RoleUser, 100)
	for _, role := range []int{1, 5, 10, 20, 100, 101} {
		got := call(SetUserFinanceRole, testRequest{method: "PUT", target: "/api/user/finance-role", role: role, body: `{"enabled":true,"expected_role":1}`})
		want := 403
		if role == 100 {
			want = 409
		}
		if got.Code != want {
			t.Fatalf("role %d: %d", role, got.Code)
		}
	}
	fresh, _ := model.GetUserByID(user.ID)
	if fresh.AccountKind != "customer" || fresh.Role != 1 || fresh.Quota != 100 {
		t.Fatal("customer account was promoted or changed")
	}
}

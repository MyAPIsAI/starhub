package controller

import (
	"fmt"
	"testing"

	"starhub/common"
	"starhub/model"
)

func TestFinanceRoleAssignmentRequiresRootAndExpectedRole(t *testing.T) {
	newTestDB(t)
	user := newUser(t, "finance-target", common.RoleUser, 100)
	change := func(actor int, body string) int {
		return call(SetUserFinanceRole, testRequest{method: "PUT", target: "/api/user/finance-role", body: body, role: actor, path: map[string]string{"id": fmt.Sprint(user.ID)}}).Code
	}
	for _, role := range []int{common.RoleUser, common.RoleFinance, common.RoleAdmin, 101} {
		if status := change(role, `{"enabled":true,"expected_role":1}`); status != 403 {
			t.Fatalf("role %d granted finance access: %d", role, status)
		}
	}
	for _, body := range []string{`{}`, `{"enabled":true}`, `{"enabled":true,"expected_role":100}`, `{"enabled":"true","expected_role":1}`} {
		if status := change(common.RoleRoot, body); status != 400 {
			t.Fatalf("invalid finance grant accepted: %s %d", body, status)
		}
	}
	if status := change(common.RoleRoot, `{"enabled":true,"expected_role":1}`); status != 200 {
		t.Fatalf("root grant failed: %d", status)
	}
	got, _ := model.GetUserByID(user.ID)
	if got.Role != common.RoleFinance || got.Quota != 100 || got.IsAdmin() {
		t.Fatalf("grant changed unrelated data or admin privilege: %+v", got)
	}
	if status := change(common.RoleRoot, `{"enabled":false,"expected_role":1}`); status != 409 {
		t.Fatalf("stale role overwrote grant: %d", status)
	}
	if status := change(common.RoleRoot, `{"enabled":false,"expected_role":5}`); status != 200 {
		t.Fatalf("root revoke failed: %d", status)
	}
	got, _ = model.GetUserByID(user.ID)
	if got.Role != common.RoleUser {
		t.Fatalf("revoke lost: %+v", got)
	}
	for _, role := range []int{common.RoleAdmin, common.RoleRoot} {
		if err := model.UpdateUserFields(user.ID, map[string]any{"role": role}); err != nil {
			t.Fatal(err)
		}
		if status := change(common.RoleRoot, `{"enabled":true,"expected_role":1}`); status != 409 {
			t.Fatalf("privileged account overwritten: role=%d status=%d", role, status)
		}
		got, _ = model.GetUserByID(user.ID)
		if got.Role != role {
			t.Fatalf("privileged role changed: %+v", got)
		}
	}
}

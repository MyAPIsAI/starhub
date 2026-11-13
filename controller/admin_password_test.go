package controller

import (
	"net/http"
	"testing"

	"starhub/common"
	"starhub/model"
)

func TestStaffPasswordChangeCompletesInitialPasswordRequirement(t *testing.T) {
	newTestDB(t)
	for _, role := range []int{common.RoleRoot, common.RoleAdmin, common.RoleFinance, common.RoleTech} {
		user := newUser(t, "staff-"+itoa(int64(role)), role, 0)
		if publicUser(user)["admin_password_change_required"] != true {
			t.Fatal("new staff account is already initialized")
		}
		r := call(UpdateSelf, testRequest{method: "PUT", target: "/api/user/self", userID: user.ID, role: role,
			body: `{"current_password":"secret","new_password":"ChangedSecret92"}`})
		expectStatus(t, r, http.StatusOK)
		fresh, err := model.GetUserByID(user.ID)
		if err != nil {
			t.Fatal(err)
		}
		if fresh.AdminPasswordChangedAt == 0 || publicUser(fresh)["admin_password_change_required"] != false {
			t.Fatal("password change did not complete initialization")
		}
		r = call(UpdateSelf, testRequest{method: "PUT", target: "/api/user/self", userID: user.ID, role: role,
			body: `{"current_password":"ChangedSecret92","new_password":"ChangedSecret92"}`})
		expectStatus(t, r, http.StatusBadRequest)
	}
}

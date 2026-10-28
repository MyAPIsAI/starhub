package common

import "testing"

func TestOnlyAssignedRolesGrantAdministrativeAuthority(t *testing.T) {
	for _, role := range []int{-1, 0, RoleUser, RoleFinance, RoleAdmin, RoleRoot, 11, 99, 101, 1000} {
		if got, want := IsAdminRole(role), role == RoleAdmin || role == RoleRoot; got != want {
			t.Fatalf("role %d admin=%v want=%v", role, got, want)
		}
	}
}

package authz

import (
	"net/http/httptest"
	"testing"

	"starhub/common"
)

func TestInventoryPolicy(t *testing.T) {
	for _, path := range []string{"/api/resource", "/api/resource/", "/api/placement", "/api/placement/resource"} {
		for _, role := range []int{common.RoleUser, common.RoleFinance, common.RoleAdmin, common.RoleRoot, 99, 101} {
			for _, method := range []string{"GET", "PUT", "DELETE"} {
				want := (role == common.RoleAdmin || role == common.RoleRoot) && method != "DELETE"
				if got := CheckRequest(role, httptest.NewRequest(method, path, nil)); got != want {
					t.Fatalf("role=%d %s %s: %v want %v", role, method, path, got, want)
				}
			}
		}
	}
}

package authz

import (
	"net/http/httptest"
	"starhub/common"
	"testing"
)

func TestServiceGroupPolicy(t *testing.T) {
	for _, path := range []string{"/api/service-group", "/api/service-group/", "/api/service-group/default/sla"} {
		for _, role := range []int{common.RoleUser, common.RoleFinance, common.RoleAdmin, common.RoleRoot, 99, 101} {
			for _, method := range []string{"GET", "PUT", "DELETE"} {
				want := role == common.RoleAdmin || role == common.RoleRoot
				if got := CheckRequest(role, httptest.NewRequest(method, path, nil)); got != want {
					t.Fatalf("service-group role=%d %s %s: %v", role, method, path, got)
				}
			}
		}
	}
}

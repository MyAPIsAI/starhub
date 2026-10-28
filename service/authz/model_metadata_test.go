package authz

import (
	"net/http/httptest"
	"testing"

	"starhub/common"
)

func TestModelMetadataPolicyDoesNotAuthorizeUnknownNumericRoles(t *testing.T) {
	for _, path := range []string{"/api/plaza/meta", "/api/plaza/meta/", "/api/plaza/meta/configuration", "/api/plaza/meta/tenant/model"} {
		for _, role := range []int{common.RoleUser, common.RoleFinance, common.RoleAdmin, common.RoleRoot, 99, 101} {
			for _, method := range []string{"GET", "PUT", "DELETE"} {
				want := role == common.RoleAdmin || role == common.RoleRoot
				if got := CheckRequest(role, httptest.NewRequest(method, path, nil)); got != want {
					t.Fatalf("metadata policy role=%d %s %s: %v", role, method, path, got)
				}
			}
		}
	}
	if !CheckRequest(common.RoleUser, httptest.NewRequest("GET", "/api/plaza/models", nil)) {
		t.Fatal("metadata policy must not make public model browsing private")
	}
}

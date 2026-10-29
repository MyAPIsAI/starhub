package authz

import (
	"net/http/httptest"
	"starhub/common"
	"testing"
)

func TestTaskPluginManagementPermissions(t *testing.T) {
	for _, role := range []int{common.RoleRoot, common.RoleAdmin, common.RoleFinance, common.RoleUser} {
		for _, method := range []string{"GET", "POST", "PUT", "DELETE"} {
			for _, path := range []string{"/api/task-plugin", "/api/task-plugin/1", "/api/task-plugin/1/test"} {
				want := (role == common.RoleRoot || role == common.RoleAdmin) && method != "DELETE"
				if got := CheckRequest(role, httptest.NewRequest(method, path, nil)); got != want {
					t.Fatalf("role=%d %s %s: got %v want %v", role, method, path, got, want)
				}
			}
		}
	}
}

package authz

import (
	"net/http/httptest"
	"testing"
)

func TestObservabilityIsTechnicalReadOnly(t *testing.T) {
	for _, path := range []string{"/api/admin/observability", "/api/admin/alerts", "/api/admin/traces/0123456789abcdef"} {
		for _, role := range []int{1, 5, 10, 20, 100, 99, 101} {
			if got := CheckRequest(role, httptest.NewRequest("GET", path, nil)); got != (role == 100 || role == 20) {
				t.Fatalf("role %d accessed %s: %v", role, path, got)
			}
		}
	}
}

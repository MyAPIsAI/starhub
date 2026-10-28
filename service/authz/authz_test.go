package authz

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"starhub/common"
)

func TestRootCanDoEverything(t *testing.T) {
	for _, res := range []Resource{ResourceUser, ResourceChannel, ResourceToken, ResourceOption, ResourceAudit} {
		for _, act := range []Action{ActionRead, ActionWrite, ActionDelete} {
			if !Allow(common.RoleRoot, res, act) {
				t.Fatalf("Root should allow %s on %s", act, res)
			}
		}
	}
}

func TestAdminCannotManageUsers(t *testing.T) {
	if Allow(common.RoleAdmin, ResourceUser, ActionWrite) {
		t.Fatal("Admin should not be able to write users (only Root can)")
	}
	if !Allow(common.RoleAdmin, ResourceUser, ActionRead) {
		t.Fatal("Admin should be able to read users")
	}
	if !Allow(common.RoleAdmin, ResourceChannel, ActionWrite) {
		t.Fatal("Admin should be able to manage channels")
	}
}

func TestUserCanOnlyManageOwnTokens(t *testing.T) {
	if !Allow(common.RoleUser, ResourceToken, ActionWrite) {
		t.Fatal("User should be able to manage own tokens")
	}
	if Allow(common.RoleUser, ResourceChannel, ActionRead) {
		t.Fatal("User should not be able to read channels")
	}
	if Allow(common.RoleUser, ResourceUser, ActionWrite) {
		t.Fatal("User should not be able to manage users")
	}
}

func TestCheckRequestRelayTrafficAlwaysAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	if !CheckRequest(common.RoleUser, req) {
		t.Fatal("relay traffic should not be subject to authz")
	}
}

func TestCheckRequestManagementAPI(t *testing.T) {
	// Admin can read channels
	req := httptest.NewRequest(http.MethodGet, "/api/channel/", nil)
	if !CheckRequest(common.RoleAdmin, req) {
		t.Fatal("Admin should be able to read channels")
	}
	// User cannot read channels
	req = httptest.NewRequest(http.MethodGet, "/api/channel/", nil)
	if CheckRequest(common.RoleUser, req) {
		t.Fatal("User should not be able to read channels")
	}
	// Admin cannot delete users
	req = httptest.NewRequest(http.MethodDelete, "/api/user/1", nil)
	if CheckRequest(common.RoleAdmin, req) {
		t.Fatal("Admin should not be able to delete users")
	}
	// Root can delete users
	req = httptest.NewRequest(http.MethodDelete, "/api/user/1", nil)
	if !CheckRequest(common.RoleRoot, req) {
		t.Fatal("Root should be able to delete users")
	}
}

func TestFinanceRoleIsReadOnlyAndHasNoAdminResourceAccess(t *testing.T) {
	if !Allow(common.RoleFinance, ResourceFinance, ActionRead) || Allow(common.RoleFinance, ResourceFinance, ActionWrite) || Allow(common.RoleFinance, ResourceFinance, ActionDelete) {
		t.Fatal("finance permissions are not read-only")
	}
	for _, resource := range []Resource{ResourceUser, ResourceChannel, ResourceOption, ResourceAudit, ResourceRedemption} {
		for _, action := range []Action{ActionRead, ActionWrite, ActionDelete} {
			if Allow(common.RoleFinance, resource, action) {
				t.Fatalf("finance role gained %s on %s", action, resource)
			}
		}
	}
	if CheckRequest(common.RoleUser, httptest.NewRequest("GET", "/api/finance/summary", nil)) {
		t.Fatal("regular user gained finance access")
	}
}

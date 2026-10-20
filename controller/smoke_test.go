package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"starhub/common"
)

// TestSmokeAllEndpoints verifies that every management API endpoint
// registered in the router responds with 200 (or a correct non-5xx)
// when called with proper authentication. This catches handler panics,
// missing route parameters, and broken DB wiring without exercising
// the full relay chain.
func TestSmokeAllEndpoints(t *testing.T) {
	newTestDB(t)
	admin := newUser(t, "admin", common.RoleAdmin, 1000000)
	user := newUser(t, "user", common.RoleUser, 100000)

	// --- User self-service ---
	expectOK(t, "GetSelf", call(GetSelf, testRequest{
		method: http.MethodGet, target: "/api/user/self",
		userID: user.ID, role: common.RoleUser,
	}))
	expectOK(t, "UserUsage", call(UserUsage, testRequest{
		method: http.MethodGet, target: "/api/user/usage?days=7",
		userID: user.ID, role: common.RoleUser,
	}))
	expectOK(t, "ListSessions", call(ListSessions, testRequest{
		method: http.MethodGet, target: "/api/user/sessions",
		userID: user.ID, role: common.RoleUser,
	}))
	expectOK(t, "ListTokens", call(ListTokens, testRequest{
		method: http.MethodGet, target: "/api/token",
		userID: user.ID, role: common.RoleUser,
	}))

	// --- Wallet & billing ---
	expectOK(t, "GetWallet", call(GetWallet, testRequest{
		method: http.MethodGet, target: "/api/wallet",
		userID: user.ID, role: common.RoleUser,
	}))
	expectOK(t, "ListLedger", call(ListLedger, testRequest{
		method: http.MethodGet, target: "/api/ledger",
		userID: user.ID, role: common.RoleUser,
	}))
	expectOK(t, "ListRechargeOrders", call(ListRechargeOrders, testRequest{
		method: http.MethodGet, target: "/api/recharge",
		userID: user.ID, role: common.RoleUser,
	}))
	expectOK(t, "ListExports", call(ListExports, testRequest{
		method: http.MethodGet, target: "/api/export",
		userID: user.ID, role: common.RoleUser,
	}))

	// --- Logs & service quality ---
	expectOK(t, "ListLogs", call(ListLogs, testRequest{
		method: http.MethodGet, target: "/api/log",
		userID: user.ID, role: common.RoleUser,
	}))
	expectOK(t, "GetServiceQuality", call(GetServiceQuality, testRequest{
		method: http.MethodGet, target: "/api/service-quality?hours=24",
		userID: user.ID, role: common.RoleUser,
	}))

	// --- Admin: user management ---
	expectOK(t, "ListUsers", call(ListUsers, testRequest{
		method: http.MethodGet, target: "/api/user",
		userID: admin.ID, role: common.RoleAdmin,
	}))
	expectOK(t, "ListAuditLogs", call(ListAuditLogs, testRequest{
		method: http.MethodGet, target: "/api/audit",
		userID: admin.ID, role: common.RoleAdmin,
	}))
	expectOK(t, "GetMetrics", call(GetMetrics, testRequest{
		method: http.MethodGet, target: "/api/metrics",
		userID: admin.ID, role: common.RoleAdmin,
	}))

	// --- Admin: channel management ---
	expectOK(t, "ListChannels", call(ListChannels, testRequest{
		method: http.MethodGet, target: "/api/channel",
		userID: admin.ID, role: common.RoleAdmin,
	}))

	// --- Admin: redemption ---
	expectOK(t, "ListRedemptions", call(ListRedemptions, testRequest{
		method: http.MethodGet, target: "/api/redemption",
		userID: admin.ID, role: common.RoleAdmin,
	}))

	// --- Admin: option management ---
	expectOK(t, "ListOptions", call(ListOptions, testRequest{
		method: http.MethodGet, target: "/api/option",
		userID: admin.ID, role: common.RoleAdmin,
	}))
	expectOK(t, "OptionStatus", call(OptionStatus, testRequest{
		method: http.MethodGet, target: "/api/option/status",
		userID: admin.ID, role: common.RoleAdmin,
	}))
	expectOK(t, "OptionHistory", call(OptionHistory, testRequest{
		method: http.MethodGet, target: "/api/option/history",
		userID: admin.ID, role: common.RoleAdmin,
	}))

	// --- Admin: service groups ---
	expectOK(t, "ListServiceGroups", call(ListServiceGroups, testRequest{
		method: http.MethodGet, target: "/api/service-group",
		userID: admin.ID, role: common.RoleAdmin,
	}))

	// --- Admin: model metadata ---
	expectOK(t, "ListModelMeta", call(ListModelMeta, testRequest{
		method: http.MethodGet, target: "/api/plaza/meta",
		userID: admin.ID, role: common.RoleAdmin,
	}))

	// --- Public: plaza (unauthenticated) ---
	expectOK(t, "PlazaListModels", call(PlazaListModels, testRequest{
		method: http.MethodGet, target: "/api/plaza/models",
	}))
	expectOK(t, "PlazaFilters", call(PlazaFilters, testRequest{
		method: http.MethodGet, target: "/api/plaza/filters",
	}))
	expectOK(t, "PlazaRank", call(PlazaRank, testRequest{
		method: http.MethodGet, target: "/api/plaza/rank?type=value",
	}))
}

func expectOK(t *testing.T, name string, rr *httptest.ResponseRecorder) {
	t.Helper()
	if rr.Code != http.StatusOK {
		t.Fatalf("smoke %s: status %d, want 200 (body %s)", name, rr.Code, rr.Body.String())
	}
}

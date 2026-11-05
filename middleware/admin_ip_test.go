package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAdminIPWhitelistNoRestriction(t *testing.T) {
	SetAdminIPWhitelist("")
	defer SetAdminIPWhitelist("")

	called := false
	handler := AdminIPRestrict(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/channel", nil)
	req.RemoteAddr = "203.0.113.99:12345"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Fatal("handler should be called when no IP whitelist is set")
	}
}

func TestAdminIPWhitelistAllowed(t *testing.T) {
	SetAdminIPWhitelist("10.0.0.0/8, 192.168.1.0/24")
	defer SetAdminIPWhitelist("")

	called := false
	handler := AdminIPRestrict(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/channel", nil)
	req.RemoteAddr = "10.0.5.3:12345"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Fatal("handler should be called for IP in whitelist")
	}
}

func TestAdminIPWhitelistBlocked(t *testing.T) {
	SetAdminIPWhitelist("10.0.0.0/8")
	defer SetAdminIPWhitelist("")

	called := false
	handler := AdminIPRestrict(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/channel", nil)
	req.RemoteAddr = "203.0.113.99:12345"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if called {
		t.Fatal("handler should NOT be called for IP outside whitelist")
	}
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestAdminIPWhitelistBareIP(t *testing.T) {
	SetAdminIPWhitelist("192.168.1.100")
	defer SetAdminIPWhitelist("")

	called := false
	handler := AdminIPRestrict(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	// Allowed IP
	req := httptest.NewRequest(http.MethodGet, "/api/channel", nil)
	req.RemoteAddr = "192.168.1.100:54321"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if !called {
		t.Fatal("bare IP should be treated as /32")
	}

	// Blocked IP
	called = false
	req2 := httptest.NewRequest(http.MethodGet, "/api/channel", nil)
	req2.RemoteAddr = "192.168.1.101:54321"
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if called {
		t.Fatal("different IP in same /24 should be blocked for bare IP rule")
	}
}

func TestAdminIPWhitelistInvalidEntries(t *testing.T) {
	SetAdminIPWhitelist("not-an-ip, 10.0.0.0/8, garbage")
	defer SetAdminIPWhitelist("")

	// Valid CIDR should still work.
	called := false
	handler := AdminIPRestrict(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/channel", nil)
	req.RemoteAddr = "10.1.2.3:9999"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if !called {
		t.Fatal("valid CIDR should work even with invalid entries in the list")
	}
}

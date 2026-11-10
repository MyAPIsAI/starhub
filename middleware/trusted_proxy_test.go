package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveClientIPNoTrustedProxies(t *testing.T) {
	ResetTrustedProxies()
	defer ResetTrustedProxies()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.5:12345"
	req.Header.Set("X-Forwarded-For", "10.0.0.1")

	ip := ResolveClientIP(req)
	if ip != "203.0.113.5" {
		t.Fatalf("expected 203.0.113.5, got %s", ip)
	}
}

func TestResolveClientIPWithTrustedProxy(t *testing.T) {
	SetTrustedProxies("10.0.0.0/8")
	defer ResetTrustedProxies()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.5")

	ip := ResolveClientIP(req)
	if ip != "203.0.113.5" {
		t.Fatalf("expected 203.0.113.5, got %s", ip)
	}
}

func TestResolveClientIPMultiHopTrustedChain(t *testing.T) {
	SetTrustedProxies("10.0.0.0/8, 192.168.0.0/16")
	defer ResetTrustedProxies()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.5, 192.168.1.1, 10.0.0.2")

	ip := ResolveClientIP(req)
	if ip != "203.0.113.5" {
		t.Fatalf("expected 203.0.113.5, got %s", ip)
	}
}

func TestResolveClientIPUntrustedRemoteAddr(t *testing.T) {
	SetTrustedProxies("10.0.0.0/8")
	defer ResetTrustedProxies()

	// RemoteAddr is not a trusted proxy, so XFF is ignored.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.99:12345"
	req.Header.Set("X-Forwarded-For", "10.0.0.1")

	ip := ResolveClientIP(req)
	if ip != "203.0.113.99" {
		t.Fatalf("expected 203.0.113.99, got %s", ip)
	}
}

func TestResolveClientIPAllHopsTrusted(t *testing.T) {
	SetTrustedProxies("10.0.0.0/8")
	defer ResetTrustedProxies()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "10.0.0.2, 10.0.0.3")

	ip := ResolveClientIP(req)
	if ip != "10.0.0.1" {
		t.Fatalf("all hops trusted — should return RemoteAddr, got %s", ip)
	}
}

func TestResolveClientIPNoXFFHeader(t *testing.T) {
	SetTrustedProxies("10.0.0.0/8")
	defer ResetTrustedProxies()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"

	ip := ResolveClientIP(req)
	if ip != "10.0.0.1" {
		t.Fatalf("expected 10.0.0.1, got %s", ip)
	}
}

func TestSetTrustedProxiesInvalidEntries(t *testing.T) {
	SetTrustedProxies("not-an-ip, 10.0.0.0/8, garbage")
	defer ResetTrustedProxies()

	// Valid CIDR should still work.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.5")

	ip := ResolveClientIP(req)
	if ip != "203.0.113.5" {
		t.Fatalf("expected 203.0.113.5, got %s", ip)
	}
}

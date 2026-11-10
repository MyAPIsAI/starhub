package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
)

// TrustedProxyConfig holds the list of trusted proxy CIDRs. When non-empty,
// the middleware resolves the client IP from X-Forwarded-For by walking
// the chain from right to left, skipping IPs that belong to trusted proxies.
// When empty (the default), X-Forwarded-For is ignored and RemoteAddr is used.
var (
	trustedProxyMu  sync.RWMutex
	trustedProxies  []*net.IPNet
	trustedProxyRaw string
)

// SetTrustedProxies parses a comma-separated CIDR list. Invalid entries are
// silently skipped. An empty string clears the list (no trusted proxies).
func SetTrustedProxies(cidrs string) {
	trustedProxyMu.Lock()
	defer trustedProxyMu.Unlock()
	trustedProxyRaw = cidrs
	trustedProxies = nil
	for _, raw := range strings.Split(cidrs, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if !strings.Contains(raw, "/") {
			ip := net.ParseIP(raw)
			if ip == nil {
				continue
			}
			if ip.To4() != nil {
				raw += "/32"
			} else {
				raw += "/128"
			}
		}
		_, cidr, err := net.ParseCIDR(raw)
		if err != nil {
			continue
		}
		trustedProxies = append(trustedProxies, cidr)
	}
}

// GetTrustedProxies returns the current configuration string.
func GetTrustedProxies() string {
	trustedProxyMu.RLock()
	defer trustedProxyMu.RUnlock()
	return trustedProxyRaw
}

// isTrustedProxy reports whether the IP belongs to a trusted proxy CIDR.
func isTrustedProxy(ip net.IP) bool {
	trustedProxyMu.RLock()
	defer trustedProxyMu.RUnlock()
	if len(trustedProxies) == 0 {
		return false
	}
	for _, cidr := range trustedProxies {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

// ResolveClientIP determines the real client IP. When trusted proxies are
// configured, it walks the X-Forwarded-For chain from right to left,
// skipping trusted proxy IPs, and returns the first untrusted IP.
// When no trusted proxies are set, it returns RemoteAddr (stripped of port).
func ResolveClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return host
	}
	// No trusted proxies configured — use RemoteAddr directly.
	if !isTrustedProxy(ip) {
		return host
	}
	// Walk X-Forwarded-For from right to left.
	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return host
	}
	hops := strings.Split(xff, ",")
	for i := len(hops) - 1; i >= 0; i-- {
		hopIP := net.ParseIP(strings.TrimSpace(hops[i]))
		if hopIP == nil {
			continue
		}
		if !isTrustedProxy(hopIP) {
			return hopIP.String()
		}
	}
	// All hops were trusted proxies — return the RemoteAddr IP.
	return host
}

// ResetTrustedProxies clears the trusted proxy list (used by tests).
func ResetTrustedProxies() {
	SetTrustedProxies("")
}

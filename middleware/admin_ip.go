package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"

	"starhub/common"
)

// AdminIPWhitelist is an in-memory CIDR allow-list for admin/dashboard
// endpoints. When configured, only requests from the listed CIDRs may
// reach AdminAuth/RootAuth/FinanceAuth handlers. An empty list means no
// restriction (the default).
var (
	adminIPMu     sync.RWMutex
	adminIPList   []*net.IPNet
	adminIPConfig string
)

// SetAdminIPWhitelist parses a comma-separated CIDR list and updates the
// active allow-list. Invalid entries are silently skipped. An empty string
// clears the list (no restriction).
func SetAdminIPWhitelist(cidrs string) {
	adminIPMu.Lock()
	defer adminIPMu.Unlock()
	adminIPConfig = cidrs
	adminIPList = nil
	for _, raw := range strings.Split(cidrs, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		// Accept bare IPs as /32 (IPv4) or /128 (IPv6).
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
		adminIPList = append(adminIPList, cidr)
	}
}

// GetAdminIPWhitelist returns the current configuration string.
func GetAdminIPWhitelist() string {
	adminIPMu.RLock()
	defer adminIPMu.RUnlock()
	return adminIPConfig
}

// AdminIPRestrict is middleware that rejects requests from IPs not in the
// admin allow-list. When no list is configured, it passes through.
func AdminIPRestrict(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		adminIPMu.RLock()
		list := adminIPList
		adminIPMu.RUnlock()
		if len(list) == 0 {
			next.ServeHTTP(w, r)
			return
		}
		ipStr := clientIPFromRequest(r)
		ip := net.ParseIP(ipStr)
		if ip == nil {
			common.ApiError(w, http.StatusForbidden, "admin access is IP-restricted; client IP could not be determined")
			return
		}
		for _, cidr := range list {
			if cidr.Contains(ip) {
				next.ServeHTTP(w, r)
				return
			}
		}
		common.ApiError(w, http.StatusForbidden, "admin access is not allowed from this IP address")
	})
}

func clientIPFromRequest(r *http.Request) string {
	return ResolveClientIP(r)
}

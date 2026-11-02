package common

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync/atomic"
)

var trustedProxies atomic.Pointer[[]netip.Prefix]

// ParseIPNetwork normalizes IPv4-mapped addresses before matching prefixes.
func ParseIPNetwork(value string) (netip.Prefix, error) {
	if address, err := netip.ParseAddr(value); err == nil && address.Zone() == "" {
		address = address.Unmap()
		return netip.PrefixFrom(address, address.BitLen()), nil
	}
	prefix, err := netip.ParsePrefix(value)
	if err != nil || prefix.Addr().Zone() != "" {
		return netip.Prefix{}, fmt.Errorf("invalid IP address or CIDR")
	}
	if prefix.Addr().Is4In6() {
		if prefix.Bits() < 96 {
			return netip.Prefix{}, fmt.Errorf("mapped IPv4 prefix must be at least /96")
		}
		prefix = netip.PrefixFrom(prefix.Addr().Unmap(), prefix.Bits()-96)
	}
	return prefix.Masked(), nil
}

func ConfigureTrustedProxies(values []string) error {
	prefixes := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		prefix, err := ParseIPNetwork(strings.TrimSpace(value))
		if err != nil {
			return fmt.Errorf("invalid trusted proxy: %w", err)
		}
		prefixes = append(prefixes, prefix)
	}
	trustedProxies.Store(&prefixes)
	return nil
}

func isTrustedProxy(address netip.Addr) bool {
	prefixes := trustedProxies.Load()
	if prefixes != nil {
		for _, prefix := range *prefixes {
			if prefix.Contains(address.Unmap()) {
				return true
			}
		}
	}
	return false
}

// ClientIP accepts forwarding headers only from a registered connection peer.
func ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	peer = peer.Unmap()
	if !isTrustedProxy(peer) {
		return peer.String()
	}
	forwarded := strings.Join(r.Header.Values("X-Forwarded-For"), ",")
	if forwarded == "" || len(forwarded) > 8192 {
		return peer.String()
	}
	parts := strings.Split(forwarded, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		address, err := netip.ParseAddr(strings.TrimSpace(parts[i]))
		if err != nil || address.Zone() != "" {
			return peer.String()
		}
		address = address.Unmap()
		if !isTrustedProxy(address) {
			return address.String()
		}
	}
	return peer.String()
}

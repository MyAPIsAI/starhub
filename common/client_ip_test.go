package common

import (
	"net/http/httptest"
	"testing"
)

func TestClientIPTrustedProxyBoundary(t *testing.T) {
	if err := ConfigureTrustedProxies([]string{"192.0.2.0/24", "2001:db8:ffff::/48"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ConfigureTrustedProxies(nil) })
	for _, test := range []struct{ peer, header, want string }{
		{"192.0.2.10:443", "203.0.113.9, 198.51.100.7", "198.51.100.7"},
		{"198.51.100.7:443", "203.0.113.9", "198.51.100.7"},
		{"192.0.2.10:443", "203.0.113.9, 192.0.2.11", "203.0.113.9"},
		{"[2001:db8:ffff::1]:443", "2001:db8::9, 192.0.2.11", "2001:db8::9"},
		{"192.0.2.10:443", "203.0.113.9, invalid", "192.0.2.10"},
		{"[::ffff:198.51.100.7]:443", "203.0.113.9", "198.51.100.7"},
		{"192.0.2.10:443", "192.0.2.11", "192.0.2.10"},
	} {
		request := httptest.NewRequest("GET", "/", nil)
		request.RemoteAddr = test.peer
		request.Header.Set("X-Forwarded-For", test.header)
		if got := ClientIP(request); got != test.want {
			t.Errorf("peer %s header %s: got %s want %s", test.peer, test.header, got, test.want)
		}
	}
}

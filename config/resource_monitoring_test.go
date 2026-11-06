package config

import (
	"net/http"
	"testing"
	"time"
)

func TestResourceMonitorTransportRequiresAuthenticatedRemoteEndpoints(t *testing.T) {
	t.Cleanup(func() { _ = ConfigureResourceMonitoring("", "", "", "", "", "") })
	for _, address := range []string{"http://example.invalid/events", "https://example.invalid/events", "http://user:password@localhost/events", "file:///tmp/events", "http://localhost/events#fragment"} {
		if err := ConfigureResourceMonitoring(address, "", "", "", "", ""); err == nil {
			t.Fatalf("unsafe or unauthenticated endpoint accepted: %s", address)
		}
	}
	if err := ConfigureResourceMonitoring("http://127.0.0.1/events", "http://localhost/snapshot", "http://[::1]/reports", "", "", ""); err != nil {
		t.Fatal(err)
	}
	client := ResourceHTTPClient(time.Second)
	if client.Timeout != time.Second || client.CheckRedirect(nil, nil) != http.ErrUseLastResponse {
		t.Fatal("monitoring transport permits unbounded requests or redirects")
	}
}

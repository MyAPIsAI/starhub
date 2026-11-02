package config

import (
	"net/http/httptest"
	"testing"

	"starhub/common"
)

func TestStartupTrustedProxiesAndRegionTimezone(t *testing.T) {
	t.Cleanup(func() { _ = ApplyStartup(Startup{}) })
	values := map[string]string{"STARHUB_TRUSTED_PROXIES": "192.0.2.0/24,2001:db8:ffff::/48", "STARHUB_REGION_TIMEZONE": "America/New_York"}
	startup, err := LoadStartup(nil, func(key string) string { return values[key] })
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyStartup(startup); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/", nil)
	request.RemoteAddr = "192.0.2.10:443"
	request.Header.Set("X-Forwarded-For", "203.0.113.9, 198.51.100.7")
	if common.ClientIP(request) != "198.51.100.7" || RegionTimezone() != "America/New_York" {
		t.Fatal("deployment security settings were not applied")
	}
	for _, bad := range []map[string]string{{"STARHUB_TRUSTED_PROXIES": "not-an-address"}, {"STARHUB_REGION_TIMEZONE": "invalid/zone"}, {"STARHUB_REGION_TIMEZONE": "Local"}} {
		if _, err := LoadStartup(nil, func(key string) string { return bad[key] }); err == nil {
			t.Fatal("invalid deployment setting accepted")
		}
	}
}

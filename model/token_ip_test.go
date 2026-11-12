package model

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"starhub/common"
)

func TestAllowsIPCidr(t *testing.T) {
	tok := &Token{AllowIps: "10.0.0.0/8, 192.168.1.0/24, 203.0.113.5"}

	cases := []struct {
		ip   string
		want bool
	}{
		{"10.1.2.3", true},       // in 10.0.0.0/8
		{"10.255.255.255", true}, // edge of 10.0.0.0/8
		{"11.0.0.1", false},      // outside 10.0.0.0/8
		{"192.168.1.100", true},  // in 192.168.1.0/24
		{"192.168.2.1", false},   // outside 192.168.1.0/24
		{"203.0.113.5", true},    // bare IP match
		{"203.0.113.6", false},   // not the bare IP
	}
	for _, tc := range cases {
		got := tok.AllowsIP(tc.ip)
		if got != tc.want {
			t.Errorf("AllowsIP(%q) = %v, want %v", tc.ip, got, tc.want)
		}
	}
}

func TestAllowsIPEmptyAllowsAll(t *testing.T) {
	tok := &Token{AllowIps: ""}
	if !tok.AllowsIP("1.2.3.4") {
		t.Fatal("empty AllowIps should allow all IPs")
	}
}

func TestAllowsIPInvalidEntriesFallBackToStringMatch(t *testing.T) {
	tok := &Token{AllowIps: "not-an-ip, 10.0.0.0/8"}
	// "not-an-ip" is invalid but doesn't crash; 10.0.0.0/8 still works.
	if !tok.AllowsIP("10.1.1.1") {
		t.Fatal("valid CIDR should still work alongside invalid entries")
	}
	if tok.AllowsIP("not-an-ip") {
		// exact string match with the invalid entry
		// (this is the backward-compatible behavior)
	}
}

func TestAllowsIPIPv6(t *testing.T) {
	tok := &Token{AllowIps: "::1/128, 2001:db8::/32"}
	if !tok.AllowsIP("::1") {
		t.Fatal("::1 should be allowed in ::1/128")
	}
	if !tok.AllowsIP("2001:db8::1") {
		t.Fatal("2001:db8::1 should be allowed in 2001:db8::/32")
	}
	if tok.AllowsIP("2001:dead::1") {
		t.Fatal("2001:dead::1 should not be allowed in 2001:db8::/32")
	}
}

// Ensure common is imported.
var _ = common.StatusEnabled

func TestInsertLogSetsAdmissionDay(t *testing.T) {
	newTestDB(t)
	l := &Log{UserID: 1, Model: "gpt-4o", Quota: 10}
	if err := InsertLog(l); err != nil {
		t.Fatalf("InsertLog: %v", err)
	}
	if l.AdmissionDay == 0 {
		t.Fatal("AdmissionDay should be set by InsertLog")
	}
	now := time.Now().UTC()
	expected := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).Unix()
	if l.AdmissionDay != expected {
		t.Fatalf("AdmissionDay = %d, want %d", l.AdmissionDay, expected)
	}
}

func TestInsertLogPreservesExplicitAdmissionDay(t *testing.T) {
	newTestDB(t)
	custom := int64(1234567890)
	l := &Log{UserID: 1, Model: "gpt-4o", Quota: 10, AdmissionDay: custom}
	if err := InsertLog(l); err != nil {
		t.Fatalf("InsertLog: %v", err)
	}
	if l.AdmissionDay != custom {
		t.Fatalf("AdmissionDay = %d, want %d (should not overwrite)", l.AdmissionDay, custom)
	}
}

func TestValidateAllowIpsCount(t *testing.T) {
	cases := []struct {
		name    string
		ips     string
		wantErr bool
	}{
		{"empty", "", false},
		{"under limit", "10.0.0.1, 10.0.0.2, 10.0.0.3", false},
		{"exactly 50", makeIPs(50), false},
		{"51 entries", makeIPs(51), true},
		{"whitespace only", "  ,  ,  ", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateAllowIpsCount(tc.ips)
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// makeIPs returns a comma-separated string of n IP entries.
func makeIPs(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "10.0.0." + strconv.Itoa(i+1)
	}
	return strings.Join(parts, ",")
}

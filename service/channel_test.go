package service

import (
	"net/http"
	"testing"
)

func TestShouldDisableChannel(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{"unauthorized", http.StatusUnauthorized, "", true},
		{"forbidden", http.StatusForbidden, "", true},
		{"invalid_api_key_code", http.StatusBadRequest,
			`{"error":{"message":"Incorrect API key provided","code":"invalid_api_key"}}`, true},
		{"keyword_in_body", http.StatusInternalServerError,
			`{"error":{"message":"the API key expired, contact support"}}`, true},
		{"account_deactivated", http.StatusBadRequest,
			`{"error":{"message":"account_deactivated"}}`, true},
		{"bad_request_plain", http.StatusBadRequest,
			`{"error":{"message":"max_tokens is too large"}}`, false},
		{"server_error_plain", http.StatusInternalServerError,
			`{"error":{"message":"upstream exploded"}}`, false},
		{"rate_limited", http.StatusTooManyRequests, "", false},
	}
	for _, c := range cases {
		if got := ShouldDisableChannel(c.status, c.body); got != c.want {
			t.Errorf("ShouldDisableChannel(%d, %q) = %v, want %v (%s)",
				c.status, c.body, got, c.want, c.name)
		}
	}
}

func TestShouldEnableChannel(t *testing.T) {
	if !ShouldEnableChannel(3) {
		t.Error("auto-disabled channels should be auto-enable candidates")
	}
	if ShouldEnableChannel(1) {
		t.Error("enabled channels need no enable")
	}
	if ShouldEnableChannel(2) {
		t.Error("manually disabled channels must stay untouched")
	}
}

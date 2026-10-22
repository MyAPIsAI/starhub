package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestContractEnvelopeShape verifies that all management API responses
// follow the declared contract envelope: { success: bool, data?: any,
// message?: string }. This is section 7 item 4 of the P21 test strategy:
// positive and negative sample checks for required field presence.
func TestContractEnvelopeShape(t *testing.T) {
	env := setupRelayApp(t)

	// Test cases: (name, method, path, body, expectSuccess)
	cases := []struct {
		name     string
		method   string
		path     string
		body     string
		anonOK   bool // true = works without auth
	}{
		{"plaza/models GET anonymous", "GET", "/api/plaza/models", "", true},
		{"plaza/filters GET anonymous", "GET", "/api/plaza/filters", "", true},
		{"plaza/rank GET anonymous", "GET", "/api/plaza/rank?type=value", "", true},
		{"user/self GET anonymous (401)", "GET", "/api/user/self", "", false},
		{"token GET anonymous (401)", "GET", "/api/token", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var token string
			if !tc.anonOK {
				token = "" // anonymous
			} else {
				token = "" // still anonymous for public endpoints
			}
			res := doRequest(env.base, tc.method, tc.path, token,
				strings.Reader{})
			_ = res

			// For anonymous requests to auth-required endpoints,
			// verify the 401 envelope shape.
			if !tc.anonOK {
				if res.Status != 401 {
					t.Fatalf("expected 401 for anonymous, got %d: %s", res.Status, res.Body)
				}
				var envelope map[string]any
				if err := json.Unmarshal([]byte(res.Body), &envelope); err != nil {
					t.Fatalf("response is not valid JSON: %v (body: %s)", err, res.Body)
				}
				// Required: success field (bool), message field (string)
				successVal, hasSuccess := envelope["success"]
				if !hasSuccess {
					t.Fatal("envelope missing required field 'success'")
				}
				if success, ok := successVal.(bool); !ok || success {
					t.Fatalf("expected success=false for 401, got %v", successVal)
				}
				msgVal, hasMsg := envelope["message"]
				if !hasMsg {
					t.Fatal("envelope missing required field 'message'")
				}
				if msg, ok := msgVal.(string); !ok || msg == "" {
					t.Fatalf("expected non-empty message, got %v", msgVal)
				}
				// Data should be absent or null for errors.
				if dataVal, hasData := envelope["data"]; hasData && dataVal != nil {
					t.Fatalf("expected no data in error envelope, got %v", dataVal)
				}
			} else {
				// For public endpoints, verify the success envelope.
				if res.Status != 200 {
					t.Fatalf("expected 200, got %d: %s", res.Status, res.Body)
				}
				var envelope map[string]any
				if err := json.Unmarshal([]byte(res.Body), &envelope); err != nil {
					t.Fatalf("response is not valid JSON: %v (body: %s)", err, res.Body)
				}
				successVal, hasSuccess := envelope["success"]
				if !hasSuccess {
					t.Fatal("envelope missing required field 'success'")
				}
				if success, ok := successVal.(bool); !ok || !success {
					t.Fatalf("expected success=true for 200, got %v", successVal)
				}
				if _, hasData := envelope["data"]; !hasData {
					t.Fatal("envelope missing required field 'data'")
				}
			}
		})
	}

	// Verify authenticated endpoints also follow the envelope.
	t.Run("user/self GET authenticated", func(t *testing.T) {
		res := doRequest(env.base, "GET", "/api/user/self", env.session, nil)
		if res.Status != 200 {
			t.Fatalf("expected 200, got %d: %s", res.Status, res.Body)
		}
		var envelope map[string]any
		if err := json.Unmarshal([]byte(res.Body), &envelope); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if success, ok := envelope["success"].(bool); !ok || !success {
			t.Fatalf("expected success=true, got %v", envelope["success"])
		}
		data, hasData := envelope["data"].(map[string]any)
		if !hasData {
			t.Fatal("data should be an object for user/self")
		}
		// Required AuthUser fields per contract.
		requiredFields := []string{"id", "username", "email", "role", "status", "quota", "group", "created_at"}
		for _, field := range requiredFields {
			if _, exists := data[field]; !exists {
				t.Fatalf("AuthUser missing required field %q", field)
			}
		}
		// Password must NOT be returned.
		if _, exists := data["password"]; exists {
			t.Fatal("AuthUser must not include 'password' field")
		}
	})

	t.Run("wallet GET authenticated", func(t *testing.T) {
		res := doRequest(env.base, "GET", "/api/wallet", env.session, nil)
		if res.Status != 200 {
			t.Fatalf("expected 200, got %d: %s", res.Status, res.Body)
		}
		var envelope map[string]any
		if err := json.Unmarshal([]byte(res.Body), &envelope); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		data, hasData := envelope["data"].(map[string]any)
		if !hasData {
			t.Fatal("data should be an object for wallet")
		}
		if _, exists := data["balance"]; !exists {
			t.Fatal("Wallet missing required field 'balance'")
		}
		if _, exists := data["user_id"]; !exists {
			t.Fatal("Wallet missing required field 'user_id'")
		}
	})

	t.Run("service-quality GET authenticated", func(t *testing.T) {
		res := doRequest(env.base, "GET", "/api/service-quality?hours=24", env.session, nil)
		if res.Status != 200 {
			t.Fatalf("expected 200, got %d: %s", res.Status, res.Body)
		}
		var envelope map[string]any
		if err := json.Unmarshal([]byte(res.Body), &envelope); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		data, _ := envelope["data"].(map[string]any)
		if data == nil {
			t.Fatal("data should be an object for service-quality")
		}
		for _, field := range []string{"window_hours", "overall", "by_model", "by_day"} {
			if _, exists := data[field]; !exists {
				t.Fatalf("ServiceQualitySummary missing required field %q", field)
			}
		}
	})

	t.Run("plaza/rank structure", func(t *testing.T) {
		res := doRequest(env.base, "GET", "/api/plaza/rank?type=value", "", nil)
		if res.Status != 200 {
			t.Fatalf("expected 200, got %d: %s", res.Status, res.Body)
		}
		var envelope map[string]any
		if err := json.Unmarshal([]byte(res.Body), &envelope); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		data, _ := envelope["data"].(map[string]any)
		if data == nil {
			t.Fatal("data should be an object for plaza/rank")
		}
		for _, field := range []string{"type", "items"} {
			if _, exists := data[field]; !exists {
				t.Fatalf("PlazaRankResponse missing required field %q", field)
			}
		}
	})

	t.Logf("contract envelope: all tested endpoints follow the {success, data, message} envelope shape")
}

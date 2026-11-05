package service

import (
	"errors"
	"testing"
)

func TestGenerateNumericVerificationCode(t *testing.T) {
	code, err := GenerateNumericVerificationCode()
	if err != nil {
		t.Fatal(err)
	}
	if len(code) != 6 {
		t.Fatalf("code length = %d, want 6", len(code))
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			t.Fatalf("code contains non-digit: %q", code)
		}
	}
	// Two codes should differ (probability of collision is 1/1e6).
	code2, _ := GenerateNumericVerificationCode()
	if code == code2 {
		t.Fatal("two consecutive codes are identical")
	}
}

func TestSMSDisabledByDefault(t *testing.T) {
	if IsSMSEnabled() {
		t.Fatal("SMS should be disabled by default")
	}
	err := SendSMSVerification("+8613812345678", "123456")
	if !errors.Is(err, ErrSMSDisabled) {
		t.Fatalf("expected ErrSMSDisabled, got %v", err)
	}
}

// mockSMSProvider captures the code for testing.
type mockSMSProvider struct {
	lastPhone string
	lastCode  string
	err       error
}

func (m *mockSMSProvider) Send(phoneE164, code string) error {
	m.lastPhone = phoneE164
	m.lastCode = code
	return m.err
}

func TestSetSMSProvider(t *testing.T) {
	mock := &mockSMSProvider{}
	SetSMSProvider(mock)
	defer SetSMSProvider(nil)

	if !IsSMSEnabled() {
		t.Fatal("SMS should be enabled after SetSMSProvider")
	}
	err := SendSMSVerification("+8613812345678", "654321")
	if err != nil {
		t.Fatalf("SendSMSVerification failed: %v", err)
	}
	if mock.lastPhone != "+8613812345678" {
		t.Fatalf("phone = %s, want +8613812345678", mock.lastPhone)
	}
	if mock.lastCode != "654321" {
		t.Fatalf("code = %s, want 654321", mock.lastCode)
	}
}

func TestSetSMSProviderNil(t *testing.T) {
	SetSMSProvider(&mockSMSProvider{})
	SetSMSProvider(nil)
	if IsSMSEnabled() {
		t.Fatal("SMS should be disabled after SetSMSProvider(nil)")
	}
}

package service

import (
	"errors"
	"testing"
)

func TestEmailDisabledByDefault(t *testing.T) {
	if IsEmailEnabled() {
		t.Fatal("email should be disabled by default")
	}
	err := SendEmailVerification("test@example.com", "123456")
	if !errors.Is(err, ErrEmailDisabled) {
		t.Fatalf("expected ErrEmailDisabled, got %v", err)
	}
}

func TestEmailProviderFailuresAreReturnedForVerificationAndNotifications(t *testing.T) {
	failure := errors.New("fixture provider unavailable")
	provider := &mockEmailProvider{err: failure}
	SetEmailProvider(provider)
	t.Cleanup(func() { SetEmailProvider(nil) })
	if err := SendEmailVerification("recipient@example.test", "654321"); !errors.Is(err, failure) {
		t.Fatal("verification failure hidden", err)
	}
	if err := SendEmailNotification("recipient@example.test", "Security event", "Fixture notification body"); !errors.Is(err, failure) {
		t.Fatal("notification failure hidden", err)
	}
	if provider.lastTo != "recipient@example.test" || provider.lastBody != "Fixture notification body" {
		t.Fatal("provider arguments changed")
	}
	SetEmailProvider(nil)
	if err := SendEmailNotification("recipient@example.test", "Security event", "Fixture notification body"); !errors.Is(err, ErrEmailDisabled) {
		t.Fatal("disabled notifications reported delivery", err)
	}
	if IsEmailEnabled() {
		t.Fatal("disabled provider remains enabled")
	}
}

type mockEmailProvider struct {
	lastTo      string
	lastCode    string
	lastSubject string
	lastBody    string
	err         error
}

func (m *mockEmailProvider) SendVerification(toEmail, code string) error {
	m.lastTo = toEmail
	m.lastCode = code
	return m.err
}

func (m *mockEmailProvider) SendNotification(toEmail, subject, body string) error {
	m.lastTo = toEmail
	m.lastSubject = subject
	m.lastBody = body
	return m.err
}

func TestSetEmailProvider(t *testing.T) {
	mock := &mockEmailProvider{}
	SetEmailProvider(mock)
	defer SetEmailProvider(nil)

	if !IsEmailEnabled() {
		t.Fatal("email should be enabled after SetEmailProvider")
	}
	if err := SendEmailVerification("test@example.com", "654321"); err != nil {
		t.Fatalf("SendEmailVerification failed: %v", err)
	}
	if mock.lastTo != "test@example.com" || mock.lastCode != "654321" {
		t.Fatalf("unexpected mock state: %+v", mock)
	}

	if err := SendEmailNotification("test@example.com", "Security Alert", "Your password was changed."); err != nil {
		t.Fatalf("SendEmailNotification failed: %v", err)
	}
	if mock.lastSubject != "Security Alert" {
		t.Fatalf("subject = %s, want Security Alert", mock.lastSubject)
	}
}

func TestSetEmailProviderNil(t *testing.T) {
	SetEmailProvider(&mockEmailProvider{})
	SetEmailProvider(nil)
	if IsEmailEnabled() {
		t.Fatal("email should be disabled after SetEmailProvider(nil)")
	}
}

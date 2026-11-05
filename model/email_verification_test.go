package model

import (
	"errors"
	"testing"
	"time"
)

func TestCreateAndConsumeEmailVerificationChallenge(t *testing.T) {
	newTestDB(t)
	email := "test@example.com"
	code := "123456"
	expiresAt := time.Now().Add(10 * time.Minute).Unix()

	if err := CreateEmailVerificationChallenge(email, code, EmailVerificationPurposeBind, expiresAt); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if err := ConsumeEmailVerificationChallenge(email, code, EmailVerificationPurposeBind); err != nil {
		t.Fatalf("Consume failed: %v", err)
	}
	// Second consume should fail.
	if err := ConsumeEmailVerificationChallenge(email, code, EmailVerificationPurposeBind); err == nil {
		t.Fatal("expected error on second consume")
	}
}

func TestConsumeEmailVerificationWrongCode(t *testing.T) {
	newTestDB(t)
	email := "wrong@example.com"
	code := "654321"
	expiresAt := time.Now().Add(10 * time.Minute).Unix()
	CreateEmailVerificationChallenge(email, code, EmailVerificationPurposeBind, expiresAt)

	err := ConsumeEmailVerificationChallenge(email, "000000", EmailVerificationPurposeBind)
	if !errors.Is(err, ErrVerificationInvalid) {
		t.Fatalf("expected ErrVerificationInvalid, got %v", err)
	}
	// Correct code still works.
	if err := ConsumeEmailVerificationChallenge(email, code, EmailVerificationPurposeBind); err != nil {
		t.Fatalf("correct code failed: %v", err)
	}
}

func TestConsumeEmailVerificationExpired(t *testing.T) {
	newTestDB(t)
	email := "expired@example.com"
	code := "222222"
	expiresAt := time.Now().Add(-1 * time.Minute).Unix()
	CreateEmailVerificationChallenge(email, code, EmailVerificationPurposeBind, expiresAt)

	err := ConsumeEmailVerificationChallenge(email, code, EmailVerificationPurposeBind)
	if !errors.Is(err, ErrVerificationInvalid) {
		t.Fatalf("expected ErrVerificationInvalid for expired, got %v", err)
	}
}

func TestResendEmailReplacesOldCode(t *testing.T) {
	newTestDB(t)
	email := "resend@example.com"
	oldCode := "444444"
	newCode := "555555"
	expiresAt := time.Now().Add(10 * time.Minute).Unix()
	CreateEmailVerificationChallenge(email, oldCode, EmailVerificationPurposeBind, expiresAt)
	CreateEmailVerificationChallenge(email, newCode, EmailVerificationPurposeBind, expiresAt)

	err := ConsumeEmailVerificationChallenge(email, oldCode, EmailVerificationPurposeBind)
	if !errors.Is(err, ErrVerificationInvalid) {
		t.Fatalf("old code should be invalid, got %v", err)
	}
	if err := ConsumeEmailVerificationChallenge(email, newCode, EmailVerificationPurposeBind); err != nil {
		t.Fatalf("new code failed: %v", err)
	}
}

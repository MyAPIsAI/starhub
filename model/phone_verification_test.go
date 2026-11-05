package model

import (
	"errors"
	"testing"
	"time"
)

func TestCreateAndConsumePhoneVerificationChallenge(t *testing.T) {
	newTestDB(t)
	phone := "+8613812345678"
	code := "123456"
	expiresAt := time.Now().Add(10 * time.Minute).Unix()

	if err := CreatePhoneVerificationChallenge(phone, code, PhoneVerificationPurposeLogin, "127.0.0.1", expiresAt); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := ConsumePhoneVerificationChallenge(phone, code, PhoneVerificationPurposeLogin); err != nil {
		t.Fatalf("Consume failed: %v", err)
	}

	// Second consume with same code should fail (already used).
	if err := ConsumePhoneVerificationChallenge(phone, code, PhoneVerificationPurposeLogin); err == nil {
		t.Fatal("expected error on second consume")
	}
}

func TestConsumePhoneVerificationChallengeWrongCode(t *testing.T) {
	newTestDB(t)
	phone := "+8613912345678"
	code := "654321"
	expiresAt := time.Now().Add(10 * time.Minute).Unix()

	CreatePhoneVerificationChallenge(phone, code, PhoneVerificationPurposeLogin, "127.0.0.1", expiresAt)

	err := ConsumePhoneVerificationChallenge(phone, "000000", PhoneVerificationPurposeLogin)
	if !errors.Is(err, ErrVerificationInvalid) {
		t.Fatalf("expected ErrVerificationInvalid, got %v", err)
	}

	// Correct code should still work after one wrong attempt.
	if err := ConsumePhoneVerificationChallenge(phone, code, PhoneVerificationPurposeLogin); err != nil {
		t.Fatalf("correct code failed after one wrong attempt: %v", err)
	}
}

func TestConsumePhoneVerificationChallengeLocked(t *testing.T) {
	newTestDB(t)
	phone := "+8613712345678"
	code := "111111"
	expiresAt := time.Now().Add(10 * time.Minute).Unix()

	CreatePhoneVerificationChallenge(phone, code, PhoneVerificationPurposeLogin, "127.0.0.1", expiresAt)

	// Make 5 wrong attempts.
	for i := 0; i < 5; i++ {
		err := ConsumePhoneVerificationChallenge(phone, "000000", PhoneVerificationPurposeLogin)
		if !errors.Is(err, ErrVerificationInvalid) {
			t.Fatalf("attempt %d: expected ErrVerificationInvalid, got %v", i, err)
		}
	}

	// 6th attempt with correct code should be locked.
	err := ConsumePhoneVerificationChallenge(phone, code, PhoneVerificationPurposeLogin)
	if !errors.Is(err, ErrVerificationLocked) {
		t.Fatalf("expected ErrVerificationLocked, got %v", err)
	}
}

func TestConsumePhoneVerificationChallengeExpired(t *testing.T) {
	newTestDB(t)
	phone := "+8613612345678"
	code := "222222"
	expiresAt := time.Now().Add(-1 * time.Minute).Unix() // already expired

	CreatePhoneVerificationChallenge(phone, code, PhoneVerificationPurposeLogin, "127.0.0.1", expiresAt)

	err := ConsumePhoneVerificationChallenge(phone, code, PhoneVerificationPurposeLogin)
	if !errors.Is(err, ErrVerificationInvalid) {
		t.Fatalf("expected ErrVerificationInvalid for expired, got %v", err)
	}
}

func TestConsumePhoneVerificationChallengeWrongPurpose(t *testing.T) {
	newTestDB(t)
	phone := "+8613512345678"
	code := "333333"
	expiresAt := time.Now().Add(10 * time.Minute).Unix()

	CreatePhoneVerificationChallenge(phone, code, PhoneVerificationPurposeRegister, "127.0.0.1", expiresAt)

	// Login purpose should not match register challenge.
	err := ConsumePhoneVerificationChallenge(phone, code, PhoneVerificationPurposeLogin)
	if !errors.Is(err, ErrVerificationInvalid) {
		t.Fatalf("expected ErrVerificationInvalid for wrong purpose, got %v", err)
	}
}

func TestResendReplacesOldCode(t *testing.T) {
	newTestDB(t)
	phone := "+8613412345678"
	oldCode := "444444"
	newCode := "555555"
	expiresAt := time.Now().Add(10 * time.Minute).Unix()

	CreatePhoneVerificationChallenge(phone, oldCode, PhoneVerificationPurposeLogin, "127.0.0.1", expiresAt)
	CreatePhoneVerificationChallenge(phone, newCode, PhoneVerificationPurposeLogin, "127.0.0.1", expiresAt)

	// Old code should be invalid.
	err := ConsumePhoneVerificationChallenge(phone, oldCode, PhoneVerificationPurposeLogin)
	if !errors.Is(err, ErrVerificationInvalid) {
		t.Fatalf("old code should be invalid after resend, got %v", err)
	}

	// New code should work.
	if err := ConsumePhoneVerificationChallenge(phone, newCode, PhoneVerificationPurposeLogin); err != nil {
		t.Fatalf("new code failed: %v", err)
	}
}

func TestIsValidPhoneVerificationCode(t *testing.T) {
	cases := []struct {
		code string
		want bool
	}{
		{"123456", true},
		{"1234", true},
		{"12345", false},
		{"12345a", false},
		{"", false},
		{"abcdef", false},
	}
	for _, tc := range cases {
		if got := IsValidPhoneVerificationCode(tc.code); got != tc.want {
			t.Errorf("IsValidPhoneVerificationCode(%q) = %v, want %v", tc.code, got, tc.want)
		}
	}
}

func TestDeletePhoneVerificationChallenges(t *testing.T) {
	newTestDB(t)
	phone := "+8613312345678"
	expiresAt := time.Now().Add(10 * time.Minute).Unix()
	CreatePhoneVerificationChallenge(phone, "666666", PhoneVerificationPurposeLogin, "127.0.0.1", expiresAt)
	CreatePhoneVerificationChallenge(phone, "777777", PhoneVerificationPurposeRegister, "127.0.0.1", expiresAt)

	if err := DeletePhoneVerificationChallenges(phone); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// No challenge should match after deletion.
	err := ConsumePhoneVerificationChallenge(phone, "666666", PhoneVerificationPurposeLogin)
	if !errors.Is(err, ErrVerificationInvalid) {
		t.Fatalf("expected ErrVerificationInvalid after delete, got %v", err)
	}
}

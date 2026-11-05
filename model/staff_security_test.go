package model

import (
	"testing"
	"time"
)

func TestStaffTOTPMatchesRFCVectors(t *testing.T) {
	// RFC 4226 Appendix D uses the ASCII secret 12345678901234567890.
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	for step, want := range []string{"755224", "287082", "359152", "969429", "338314", "254676", "287922", "162583", "399871", "520489"} {
		got, err := TOTPCode(secret, int64(step))
		if err != nil || got != want {
			t.Fatalf("counter %d: %s %v, want %s", step, got, err, want)
		}
	}
}

func TestStaffTOTPRejectsReplayAndThrottlesAcrossSessions(t *testing.T) {
	newTestDB(t)
	user := &User{Username: "staff", Role: 100, Status: 1, Password: "unused", AffCode: "staff"}
	if err := CreateUser(user); err != nil {
		t.Fatal(err)
	}
	if err := DB.Model(&StaffAccount{}).Where("id = ?", user.ID).Update("must_change_password", false).Error; err != nil {
		t.Fatal(err)
	}
	secret, _ := NewTOTPSecret()
	now := time.Now()
	code, _ := TOTPCode(secret, now.Unix()/30)
	if _, err := VerifyStaffOTP(user.ID, 0, code, secret, now); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyStaffOTP(user.ID, 0, code, "", now); err == nil {
		t.Fatal("replayed code accepted")
	}
	row, _ := GetUserByID(user.ID)
	if row.TOTPSecret == secret || row.TOTPSecret == "" {
		t.Fatal("TOTP seed was not encrypted")
	}
	for i := 0; i < 4; i++ {
		_, _ = VerifyStaffOTP(user.ID, 0, "invalid", "", now)
	}
	next := now.Add(30 * time.Second)
	nextCode, _ := TOTPCode(secret, next.Unix()/30)
	if _, err := VerifyStaffOTP(user.ID, 0, nextCode, "", next); err == nil {
		t.Fatal("account-wide OTP throttle was bypassed")
	}
}

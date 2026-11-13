package model

import (
	"testing"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"starhub/common"
)

func TestTwoFASetupAndEnable(t *testing.T) {
	newTestDB(t)
	user := &User{Username: "2fa-test", Password: "x", Role: 1, Status: 1, Quota: 0, Group: "default", AffCode: common.GenerateAffCode()}
	if err := CreateUser(user); err != nil {
		t.Fatal(err)
	}

	// No 2FA initially
	twofa, err := GetTwoFA(user.ID)
	if err != Err2FANotEnabled {
		t.Fatalf("expected Err2FANotEnabled, got %v", err)
	}

	// Setup
	key, err := common.GenerateTOTPSecret(user.Username)
	if err != nil {
		t.Fatal(err)
	}
	if err := CreateTwoFASetup(user.ID, key.Secret()); err != nil {
		t.Fatal(err)
	}

	// Status: not yet enabled
	twofa, _ = GetTwoFA(user.ID)
	if twofa.IsEnabled {
		t.Fatal("2FA should not be enabled yet")
	}

	// Enable with wrong code
	_, err = EnableTwoFA(user.ID, "000000")
	if err != Err2FAInvalidCode {
		t.Fatalf("expected Err2FAInvalidCode, got %v", err)
	}

	// Enable with correct code (generate a valid TOTP)
	code := generateValidTOTP(t, key.Secret())
	codes, err := EnableTwoFA(user.ID, code)
	if err != nil {
		t.Fatalf("EnableTwoFA failed: %v", err)
	}
	if len(codes) != 4 {
		t.Fatalf("expected 4 backup codes, got %d", len(codes))
	}

	// Status: enabled
	twofa, _ = GetTwoFA(user.ID)
	if !twofa.IsEnabled {
		t.Fatal("2FA should be enabled")
	}
}

func TestTwoFAVerify(t *testing.T) {
	newTestDB(t)
	user := &User{Username: "2fa-verify", Password: "x", Role: 1, Status: 1, Quota: 0, Group: "default", AffCode: common.GenerateAffCode()}
	CreateUser(user)

	key, _ := common.GenerateTOTPSecret(user.Username)
	CreateTwoFASetup(user.ID, key.Secret())
	code := generateValidTOTP(t, key.Secret())
	EnableTwoFA(user.ID, code)

	// Correct code
	validCode := generateValidTOTP(t, key.Secret())
	if err := VerifyTwoFA(user.ID, validCode); err != nil {
		t.Fatalf("VerifyTwoFA with correct code failed: %v", err)
	}

	// Wrong code
	if err := VerifyTwoFA(user.ID, "000000"); err != Err2FAInvalidCode {
		t.Fatalf("expected Err2FAInvalidCode, got %v", err)
	}
}

func TestTwoFADisable(t *testing.T) {
	newTestDB(t)
	user := &User{Username: "2fa-disable", Password: "x", Role: 1, Status: 1, Quota: 0, Group: "default", AffCode: common.GenerateAffCode()}
	CreateUser(user)

	key, _ := common.GenerateTOTPSecret(user.Username)
	CreateTwoFASetup(user.ID, key.Secret())
	code := generateValidTOTP(t, key.Secret())
	EnableTwoFA(user.ID, code)

	// Disable with wrong code
	if err := DisableTwoFA(user.ID, "000000"); err == nil {
		t.Fatal("disable with wrong code should fail")
	}

	// Disable with correct code
	validCode := generateValidTOTP(t, key.Secret())
	if err := DisableTwoFA(user.ID, validCode); err != nil {
		t.Fatalf("DisableTwoFA failed: %v", err)
	}

	twofa, _ := GetTwoFA(user.ID)
	if twofa.IsEnabled {
		t.Fatal("2FA should be disabled")
	}
}

func TestTwoFALockout(t *testing.T) {
	newTestDB(t)
	user := &User{Username: "2fa-lock", Password: "x", Role: 1, Status: 1, Quota: 0, Group: "default", AffCode: common.GenerateAffCode()}
	CreateUser(user)

	key, _ := common.GenerateTOTPSecret(user.Username)
	CreateTwoFASetup(user.ID, key.Secret())
	code := generateValidTOTP(t, key.Secret())
	EnableTwoFA(user.ID, code)

	// Make 5 wrong attempts
	for i := 0; i < 5; i++ {
		_ = VerifyTwoFA(user.ID, "000000")
	}

	// Correct code should now be locked
	validCode := generateValidTOTP(t, key.Secret())
	err := VerifyTwoFA(user.ID, validCode)
	if err != Err2FALocked {
		t.Fatalf("expected Err2FALocked after 5 failures, got %v", err)
	}
}

func TestTwoFAEnrollmentCannotReplaceAnEnabledFactor(t *testing.T) {
	newTestDB(t)
	user := &User{Username: "protected-factor", Role: common.RoleAdmin, Status: 1, AffCode: common.GenerateAffCode()}
	if err := CreateUser(user); err != nil {
		t.Fatal(err)
	}
	key, err := common.GenerateTOTPSecret(user.Username)
	if err != nil {
		t.Fatal(err)
	}
	if err := CreateTwoFASetup(user.ID, key.Secret()); err != nil {
		t.Fatal(err)
	}
	code := generateValidTOTP(t, key.Secret())
	if _, err := EnableTwoFA(user.ID, code); err != ErrStaffPasswordRequired {
		t.Fatalf("initial password guard: %v", err)
	}
	if err := UpdateUserFields(user.ID, map[string]any{"admin_password_changed_at": 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := EnableTwoFA(user.ID, code); err != nil {
		t.Fatal(err)
	}
	if err := CreateTwoFASetup(user.ID, "replacement"); err != Err2FAAlreadyOn {
		t.Fatalf("enabled factor replacement: %v", err)
	}
	if err := DisableTwoFA(user.ID, code); err != ErrStaffFactorRequired {
		t.Fatalf("staff factor disable: %v", err)
	}
	factor, err := GetTwoFA(user.ID)
	if err != nil || !factor.IsEnabled || factor.Secret != key.Secret() {
		t.Fatal("staff factor changed")
	}
}

func TestTwoFABackupDisableIsAtomicAndSingleUse(t *testing.T) {
	newTestDB(t)
	user := &User{Username: "backup-disable", Role: common.RoleUser, Status: 1, AffCode: common.GenerateAffCode()}
	if err := CreateUser(user); err != nil {
		t.Fatal(err)
	}
	key, err := common.GenerateTOTPSecret(user.Username)
	if err != nil {
		t.Fatal(err)
	}
	if err := CreateTwoFASetup(user.ID, key.Secret()); err != nil {
		t.Fatal(err)
	}
	codes, err := EnableTwoFA(user.ID, generateValidTOTP(t, key.Secret()))
	if err != nil {
		t.Fatal(err)
	}
	if err := DisableTwoFA(user.ID, "invalid-backup"); err == nil {
		t.Fatal("invalid code disabled factor")
	}
	factor, _ := GetTwoFA(user.ID)
	if !factor.IsEnabled {
		t.Fatal("failed disable did not roll back")
	}
	if err := DisableTwoFA(user.ID, codes[0]); err != nil {
		t.Fatal(err)
	}
	factor, _ = GetTwoFA(user.ID)
	if factor.IsEnabled {
		t.Fatal("backup code did not disable factor")
	}
	if err := DB.Model(&TwoFA{}).Where("user_id = ?", user.ID).Update("is_enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := DisableTwoFA(user.ID, codes[0]); err == nil {
		t.Fatal("backup code was reused")
	}
}

// generateValidTOTP uses the same library to produce a valid code for testing.
func generateValidTOTP(t *testing.T, secret string) string {
	t.Helper()
	code, err := generateTOTPCode(secret)
	if err != nil {
		t.Fatal(err)
	}
	return code
}

func generateTOTPCode(secret string) (string, error) {
	return totp.GenerateCodeCustom(secret, time.Now(), totp.ValidateOpts{
		Period:    30,
		Skew:      0,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
}

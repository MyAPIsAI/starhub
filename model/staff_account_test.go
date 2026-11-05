package model

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"gorm.io/gorm"
	"starhub/common"
)

func TestStaffIdentityMigrationPreservesCredentialsAndBusinessOwnership(t *testing.T) {
	path := filepath.Join(t.TempDir(), "split.db")
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	hash, _ := common.HashPassword("Staff-password-123")
	staff := &User{Username: "same-name", Password: hash, Role: 100, Status: 1, AffCode: "staff", Quota: 7123, UsedQuota: 456, QuotaNanoRemainder: 789}
	if err := CreateUser(staff); err != nil {
		t.Fatal(err)
	}
	secret, _ := NewTOTPSecret()
	sealed, err := sealStaffSecret(DB, staff.ID, secret)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	legacyFields := map[string]any{"password": hash, "must_change_password": false, "totp_secret": sealed, "totp_last_step": now.Unix()/30 - 1, "otp_failures": 2, "security_version": 7}
	if err := DB.Model(&User{}).Where("id = ?", staff.ID).Updates(legacyFields).Error; err != nil {
		t.Fatal(err)
	}
	key := &Token{UserID: staff.ID, Name: "Preserved staff key", UnlimitedQuota: true}
	if err := InsertToken(key); err != nil {
		t.Fatal(err)
	}
	wallet := Wallet{UserID: staff.ID, Balance: 123456789}
	ledger := LedgerEntry{UserID: staff.ID, Type: "adjustment", Amount: 123456789, BalanceAfter: 123456789}
	audit := AuditLog{UserID: staff.ID, TargetID: staff.ID, Username: staff.Username, ActorRole: 100, Action: "staff_created"}
	for _, row := range []any{&wallet, &ledger, &audit} {
		if err := DB.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := DB.Migrator().DropTable(&StaffAccount{}); err != nil {
		t.Fatal(err)
	}
	if err := DB.Exec("CREATE UNIQUE INDEX idx_users_username ON users(username)").Error; err != nil {
		t.Fatal(err)
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	identity, err := GetStaffByUsername(staff.Username)
	if err != nil {
		t.Fatal(err)
	}
	if identity.ID != staff.ID || identity.Password != hash || identity.TOTPSecret != sealed || identity.MustChangePassword || identity.SecurityVersion != 7 || identity.OTPFailures != 2 || identity.Quota != staff.Quota || identity.UsedQuota != staff.UsedQuota || identity.QuotaNanoRemainder != staff.QuotaNanoRemainder {
		t.Fatal("migration changed employee credentials or business ownership")
	}
	var raw User
	if err := DB.First(&raw, staff.ID).Error; err != nil {
		t.Fatal(err)
	}
	if raw.Password != "" || raw.TOTPSecret != "" || raw.TOTPLastStep != 0 || raw.OTPFailures != 0 || raw.SecurityVersion != 0 {
		t.Fatal("staff credentials remain in users")
	}
	if _, err := GetUserByUsername(staff.Username); !errors.Is(err, ErrUserNotFound) {
		t.Fatal("employee owner became a customer login")
	}
	retainedKey, err := GetTokenByID(key.ID)
	if err != nil || retainedKey.Key != key.Key || retainedKey.UserID != staff.ID {
		t.Fatal("key ownership or key material changed")
	}
	var retainedWallet Wallet
	var retainedLedger LedgerEntry
	var retainedAudit AuditLog
	DB.First(&retainedWallet, wallet.UserID)
	DB.First(&retainedLedger, ledger.ID)
	DB.First(&retainedAudit, audit.ID)
	if !reflect.DeepEqual(wallet, retainedWallet) || !reflect.DeepEqual(ledger, retainedLedger) || !reflect.DeepEqual(audit, retainedAudit) {
		t.Fatal("business or audit records changed")
	}
	code, _ := TOTPCode(secret, now.Unix()/30)
	if _, err := VerifyStaffOTP(staff.ID, 7, code, "", now); err != nil {
		t.Fatal("existing authenticator no longer works:", err)
	}
	changed, err := ChangeStaffPassword(staff.ID, 7, "Staff-password-123", "Rotated-staff-password-123")
	if err != nil {
		t.Fatal(err)
	}
	customer := &User{Username: staff.Username, Password: "independent-customer-hash", Role: 1, Status: 1, AffCode: "customer"}
	if err := CreateUser(customer); err != nil {
		t.Fatal("separate realms cannot share a username:", err)
	}
	if customer.ID == staff.ID {
		t.Fatal("separate principals share ownership")
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	fresh, err := GetStaffByUsername(staff.Username)
	if err != nil || fresh.Password != changed.Password || fresh.TOTPSecret != sealed || fresh.SecurityVersion != 8 || fresh.MustChangePassword || fresh.TOTPLastStep != now.Unix()/30 {
		t.Fatal("migration rerun restored old security state")
	}
	member, err := GetUserByUsername(staff.Username)
	if err != nil || member.ID != customer.ID || member.Password != customer.Password {
		t.Fatal("customer credentials changed")
	}
}

func TestStaffIdentityMigrationRollsBackBeforeScrubbing(t *testing.T) {
	newTestDB(t)
	staff := &User{Username: "rollback-staff", Password: "original-hash", Role: 100, AffCode: "rollback"}
	if err := CreateUser(staff); err != nil {
		t.Fatal(err)
	}
	if staff.Status != common.StatusEnabled || !staff.MustChangePassword {
		t.Fatal("new staff identity lost account defaults")
	}
	if err := DB.Model(&User{}).Where("id = ?", staff.ID).Update("password", staff.Password).Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.Delete(&StaffAccount{}, staff.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.Callback().Update().Before("gorm:update").Register("fail_staff_scrub", func(tx *gorm.DB) {
		if tx.Statement.Table == "users" {
			tx.AddError(errors.New("injected migration failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	err := migrateStaffIdentities(DB)
	DB.Callback().Update().Remove("fail_staff_scrub")
	if err == nil {
		t.Fatal("injected migration failure ignored")
	}
	var count int64
	DB.Model(&StaffAccount{}).Count(&count)
	var raw User
	DB.First(&raw, staff.ID)
	if count != 0 || raw.Password != staff.Password {
		t.Fatal("failed migration partially committed")
	}
	if err := migrateStaffIdentities(DB); err != nil {
		t.Fatal(err)
	}
	identity, err := GetStaffByID(staff.ID)
	if err != nil || identity.Password != staff.Password {
		t.Fatal("retry lost credentials")
	}
}

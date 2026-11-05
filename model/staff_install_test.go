package model

import (
	"path/filepath"
	"starhub/common"
	"sync"
	"testing"
)

func TestStaffInstallationTokenIsSingleUseAndRecoveryKeepsIdentity(t *testing.T) {
	newTestDB(t)
	token, err := NewStaffInstallationToken()
	if err != nil {
		t.Fatal(err)
	}
	if err := InitializeStaffRoot("root", "Temporary-root-123", "incorrect", true); err == nil {
		t.Fatal("invalid installation token admitted")
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- InitializeStaffRoot("root", "Temporary-root-123", token, true) }()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("concurrent installations succeeded %d times", success)
	}
	root, _ := GetStaffByUsername("root")
	version := root.SecurityVersion
	if err := ResetSoleRootOTP("root"); err != nil {
		t.Fatal(err)
	}
	fresh, _ := GetUserByID(root.ID)
	if fresh.SecurityVersion != version+1 || fresh.Password != root.Password || fresh.AccountKind != "staff" {
		t.Fatal("sole Root recovery changed identity or did not revoke sessions")
	}
	if err := UpdateStaffAccount(root.ID, fresh.SecurityVersion, common.RoleAdmin, 1, false, 0, "Cannot remove last root"); err != ErrLastRoot {
		t.Fatal("last Root demotion admitted")
	}
	other := &User{Username: "other-root", Role: 100, Status: 1, Password: "fixture", AffCode: "other-root"}
	if err := CreateStaffAccount(other, root.ID, "Root redundancy"); err != nil {
		t.Fatal(err)
	}
	if err := ResetSoleRootOTP("root"); err == nil {
		t.Fatal("CLI recovery allowed despite another active Root")
	}
	var count int64
	DB.Model(&AuditLog{}).Where("action = ?", "staff_2fa_reset").Count(&count)
	if count != 1 {
		t.Fatal("2FA recovery was not audited")
	}
}

func TestLegacyStaffMigrationPreservesPasswordsKeysAndCustomerBalances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	root := &User{Username: "legacy-root", Role: 100, Status: 1, Password: "unchanged-hash", Quota: 12345, AffCode: "legacy-root"}
	customer := &User{Username: "legacy-customer", Role: 1, Status: 1, Password: "customer-hash", Quota: 67890, AffCode: "legacy-customer"}
	if err := CreateUser(root); err != nil {
		t.Fatal(err)
	}
	if err := CreateUser(customer); err != nil {
		t.Fatal(err)
	}
	key := &Token{UserID: root.ID, Name: "Retained legacy key", Status: 1, UnlimitedQuota: true}
	if err := InsertToken(key); err != nil {
		t.Fatal(err)
	}
	originalKey := key.Key
	// Reconstruct the old schema, where credentials lived on the owner row.
	if err := DB.Model(&User{}).Where("id = ?", root.ID).Updates(map[string]any{"password": root.Password}).Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.Migrator().DropTable(&StaffAccount{}); err != nil {
		t.Fatal(err)
	}
	if err := DB.Migrator().DropIndex(&User{}, "idx_users_realm_username"); err != nil {
		t.Fatal(err)
	}
	if err := DB.Exec("CREATE UNIQUE INDEX idx_users_username ON users(username)").Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.Migrator().DropColumn(&User{}, "AccountKind"); err != nil {
		t.Fatal(err)
	}
	_ = CloseDB()
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	fresh, _ := GetUserByID(root.ID)
	member, _ := GetUserByID(customer.ID)
	retained, _ := GetTokenByID(key.ID)
	if fresh.AccountKind != "staff" || !fresh.MustChangePassword || fresh.Password != root.Password || fresh.Quota != root.Quota || fresh.SecurityVersion != 1 {
		t.Fatal("legacy privileged identity migration failed")
	}
	if member.AccountKind != "customer" || member.MustChangePassword || member.Quota != customer.Quota || retained.Key != originalKey {
		t.Fatal("migration changed customer money or retained Key")
	}
	if err := DB.Model(&StaffAccount{}).Where("id = ?", root.ID).Update("must_change_password", false).Error; err != nil {
		t.Fatal(err)
	}
	_ = CloseDB()
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	fresh, _ = GetUserByID(root.ID)
	if fresh.MustChangePassword || fresh.SecurityVersion != 1 {
		t.Fatal("staff migration repeated on reopen")
	}
}

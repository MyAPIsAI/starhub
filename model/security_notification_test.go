package model

import (
	"testing"

	"gorm.io/gorm"
)

func TestSecurityNotificationFreezesTrustedRecipientAndRollsBack(t *testing.T) {
	newTestDB(t)
	u := User{Username: "notification", Password: "old-hash", Email: "trusted@example.test", EmailVerifiedAt: 1, Language: "ar", AffCode: "notification"}
	if err := CreateUser(&u); err != nil {
		t.Fatal(err)
	}
	if err := updateUserSecurityFields(u.ID, map[string]any{"email": "untrusted@example.test", "password": "new-hash"}); err != nil {
		t.Fatal(err)
	}
	var rows []SecurityNotification
	if err := DB.Find(&rows).Error; err != nil || len(rows) != 2 {
		t.Fatal("missing atomic notifications", rows, err)
	}
	for _, row := range rows {
		if row.Email != u.Email || row.Language != "ar" {
			t.Fatal("recipient not frozen", row)
		}
	}
	after, _ := GetUserByID(u.ID)
	if after.EmailVerifiedAt != 0 {
		t.Fatal("unverified address remained trusted")
	}
	if err := updateUserSecurityFields(u.ID, map[string]any{"password": "third-hash"}); err != nil {
		t.Fatal(err)
	}
	var count int64
	DB.Model(&SecurityNotification{}).Count(&count)
	if count != 2 {
		t.Fatal("unverified recipient notified")
	}
	if err := DB.Model(&User{}).Where("id = ?", u.ID).Updates(map[string]any{"email": u.Email, "email_verified_at": 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.Callback().Create().Before("gorm:create").Register("notification_failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "security_notifications" {
			tx.AddError(ErrUserProfileConflict)
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = DB.Callback().Create().Remove("notification_failure") })
	if err := updateUserSecurityFields(u.ID, map[string]any{"password": "must-rollback"}); err == nil {
		t.Fatal("notification failure ignored")
	}
	after, _ = GetUserByID(u.ID)
	if after.Password != "third-hash" {
		t.Fatal("password committed without notification")
	}
}

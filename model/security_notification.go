package model

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SecurityNotification freezes only verified contact and language metadata.
// It never stores passwords, verification codes or request content.
type SecurityNotification struct {
	ID          string `gorm:"primaryKey;size:64" json:"id"`
	UserID      int64  `gorm:"index;not null" json:"-"`
	Email       string `gorm:"size:64;not null" json:"-"`
	Language    string `gorm:"size:8;not null" json:"language"`
	Action      string `gorm:"size:32;not null" json:"action"`
	Status      string `gorm:"size:16;index:idx_security_delivery,priority:1;not null" json:"status"`
	Attempts    int    `gorm:"not null" json:"attempts"`
	NextAttempt int64  `gorm:"index:idx_security_delivery,priority:2;not null" json:"-"`
	LastError   string `gorm:"size:64;not null" json:"last_error,omitempty"`
	CreatedAt   int64  `gorm:"autoCreateTime;index" json:"created_at"`
}

func queueSecurityNotifications(tx *gorm.DB, before User, fields map[string]any) error {
	for _, field := range []string{"password", "email"} {
		value, changed := fields[field].(string)
		previous := before.Password
		if field == "email" {
			previous = before.Email
		}
		if !changed || value == previous {
			continue
		}
		if field == "email" {
			if _, verified := fields["email_verified_at"]; !verified {
				fields["email_verified_at"] = int64(0)
			}
		}
		if before.Email == "" || before.EmailVerifiedAt == 0 {
			continue
		}
		language := before.Language
		if language == "" {
			day, err := captureBusinessDay(tx, time.Now())
			if err != nil {
				return err
			}
			if day.BusinessRegion != "" {
				var profile RegionProfile
				if err := tx.First(&profile, "id = ?", day.BusinessRegion).Error; err != nil {
					return err
				}
				language = profile.ResolveLanguage("", "")
			}
		}
		if language == "" {
			language = "en"
		}
		row := SecurityNotification{ID: uuid.NewString(), UserID: before.ID, Email: before.Email, Language: language, Action: field + "_changed", Status: "pending", NextAttempt: time.Now().Unix()}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func updateUserSecurityFields(id int64, fields map[string]any, expectedPassword ...string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&User{}).Where("id = ?", id).UpdateColumn("id", gorm.Expr("id")).Error; err != nil {
			return err
		}
		var before User
		if err := tx.First(&before, "id = ?", id).Error; err != nil {
			return err
		}
		if len(expectedPassword) > 0 && (len(expectedPassword) != 1 || before.Password != expectedPassword[0]) {
			return ErrUserProfileConflict
		}
		if err := queueSecurityNotifications(tx, before, fields); err != nil {
			return err
		}
		return tx.Model(&User{}).Where("id = ?", id).Updates(fields).Error
	})
}

// UpdateVerifiedEmail requires the password proof observed before code consumption.
func UpdateVerifiedEmail(id int64, password, email string) error {
	return updateUserSecurityFields(id, map[string]any{"email": email, "email_verified_at": time.Now().Unix()}, password)
}

// CompleteSecurityNotification only acknowledges the version selected by a worker.
func CompleteSecurityNotification(row SecurityNotification, accepted bool) error {
	return CompleteSecurityNotificationContext(context.Background(), row, accepted)
}

func CompleteSecurityNotificationContext(ctx context.Context, row SecurityNotification, accepted bool) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	status, message, attempts := "pending", "provider_unavailable", row.Attempts+1
	if accepted {
		status, message = "accepted", ""
	} else if attempts >= 8 {
		status = "failed"
	}
	result := DB.WithContext(ctx).Model(&SecurityNotification{}).Where("id = ? AND status = ? AND attempts = ?", row.ID, "pending", row.Attempts).
		Updates(map[string]any{"status": status, "last_error": message, "attempts": attempts, "next_attempt": time.Now().Add(time.Duration(1<<min(attempts, 8)) * 30 * time.Second).Unix()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("notification state changed")
	}
	return nil
}

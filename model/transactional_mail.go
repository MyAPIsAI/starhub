package model

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// TransactionalMail freezes the recipient and language at the committed event.
type TransactionalMail struct {
	ID          int64  `gorm:"primaryKey"`
	UserID      int64  `gorm:"index;not null"`
	Recipient   string `gorm:"not null"`
	Language    string `gorm:"size:8;not null"`
	Kind        string `gorm:"size:32;not null"`
	Status      string `gorm:"index;not null;default:pending"`
	Attempts    int
	AvailableAt int64
	LeaseUntil  int64
	SentAt      int64
	CreatedAt   int64 `gorm:"autoCreateTime"`
}

func queueProfileMail(tx *gorm.DB, user *User, kind string) error {
	if user.Email == "" {
		return nil
	}
	// Resolve the policy in the same transaction as the profile event.
	var current User
	if err := tx.Select("preferred_language").First(&current, user.ID).Error; err != nil {
		return err
	}
	language := current.PreferredLanguage
	var row DeploymentProfileVersion
	err := tx.Order("revision DESC").First(&row).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}
	profile := row.Profile
	if err == gorm.ErrRecordNotFound {
		profile = DefaultDeploymentProfile()
	}
	language = effectivePolicyLanguage(localizationPolicyForProfile(profile), language)
	return tx.Create(&TransactionalMail{UserID: user.ID, Recipient: user.Email, Language: language, Kind: kind, Status: "pending"}).Error
}

func RenderTransactionalMail(kind, language string) (string, string, error) {
	type text struct{ subject, body string }
	templates := map[string]map[string]text{
		"password_changed": {
			"en": {"Your account password was changed", "Your account password was changed successfully. If you did not make this change, contact support immediately."},
			"zh": {"您的账号密码已修改", "您的账号密码已成功修改。如果这不是您本人的操作，请立即联系支持人员。"},
			"ar": {"تم تغيير كلمة مرور حسابك", "تم تغيير كلمة مرور حسابك بنجاح. إذا لم تقم بهذا التغيير، فتواصل مع الدعم فورًا."},
		},
		"email_changed": {
			"en": {"Your account email was changed", "The email address on your account was changed. If you did not make this change, contact support immediately."},
			"zh": {"您的账号邮箱已修改", "您的账号邮箱已修改。如果这不是您本人的操作，请立即联系支持人员。"},
			"ar": {"تم تغيير بريد حسابك الإلكتروني", "تم تغيير عنوان البريد الإلكتروني لحسابك. إذا لم تقم بهذا التغيير، فتواصل مع الدعم فورًا."},
		},
	}
	texts, ok := templates[kind]
	if !ok {
		return "", "", fmt.Errorf("unsupported transactional mail")
	}
	value, ok := texts[language]
	if !ok {
		value = texts["en"]
	}
	return value.subject, value.body, nil
}

// DeliverTransactionalMail uses a bounded claim and preserves failed deliveries.
func DeliverTransactionalMail(ctx context.Context, now time.Time, send func(context.Context, TransactionalMail) error) error {
	started := time.Now()
	var rows []TransactionalMail
	if err := DB.WithContext(ctx).Where("status = ? AND available_at <= ? AND lease_until <= ?", "pending", now.Unix(), now.Unix()).Order("id").Limit(20).Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		claimedAt := now.Add(time.Since(started))
		lease := claimedAt.Add(time.Minute).Unix()
		claim := DB.WithContext(ctx).Model(&TransactionalMail{}).Where("id = ? AND status = ? AND available_at <= ? AND lease_until <= ?", row.ID, "pending", claimedAt.Unix(), claimedAt.Unix()).Updates(map[string]any{"lease_until": lease, "attempts": gorm.Expr("attempts + 1")})
		if claim.Error != nil {
			return claim.Error
		}
		if claim.RowsAffected != 1 {
			continue
		}
		err := send(ctx, row)
		finishedAt := now.Add(time.Since(started))
		values := map[string]any{"lease_until": 0, "available_at": finishedAt.Add(time.Minute).Unix()}
		if err == nil {
			values["status"] = "sent"
			values["sent_at"] = finishedAt.Unix()
		}
		if err := DB.WithContext(ctx).Model(&TransactionalMail{}).Where("id = ? AND status = ? AND lease_until = ?", row.ID, "pending", lease).Updates(values).Error; err != nil {
			return err
		}
	}
	return nil
}

package model

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"starhub/common"
)

const (
	EmailVerificationPurposeBind = "email_bind"
)

// EmailVerificationChallenge stores an outstanding email verification code.
// Same HMAC pattern as PhoneVerificationChallenge: the code is never stored
// in plaintext.
type EmailVerificationChallenge struct {
	ID           int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	Email        string `gorm:"size:64;not null;index" json:"email"`
	Purpose      string `gorm:"size:32;not null;index" json:"purpose"`
	CodeHash     string `gorm:"size:128;not null" json:"-"` // HMAC-SHA256
	ExpiresAt    int64  `gorm:"not null;index" json:"expires_at"`
	ConsumedAt   *int64 `json:"consumed_at,omitempty"`
	AttemptCount int    `gorm:"not null;default:0" json:"attempt_count"`
	CreatedAt    int64  `gorm:"autoCreateTime;index" json:"created_at"`
}

// CreateEmailVerificationChallenge creates a new challenge, replacing any
// existing unconsumed one for the same (email, purpose) pair.
func CreateEmailVerificationChallenge(email, code, purpose string, expiresAt int64) error {
	codeHash := common.GenerateHMAC("email-verification:" + email + ":" + code)
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("email = ? AND purpose = ? AND consumed_at IS NULL",
			email, purpose).Delete(&EmailVerificationChallenge{}).Error; err != nil {
			return err
		}
		return tx.Create(&EmailVerificationChallenge{
			Email:     email,
			Purpose:   purpose,
			CodeHash:  codeHash,
			ExpiresAt: expiresAt,
		}).Error
	})
}

// ConsumeEmailVerificationChallenge verifies and consumes a challenge.
// Failed attempt counters persist independently of the caller's transaction.
func ConsumeEmailVerificationChallenge(email, code, purpose string) error {
	codeHash := common.GenerateHMAC("email-verification:" + email + ":" + code)
	now := time.Now().Unix()

	var challenge EmailVerificationChallenge
	err := DB.Where("email = ? AND purpose = ? AND consumed_at IS NULL AND expires_at > ?",
		email, purpose, now).
		Order("id DESC").First(&challenge).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrVerificationInvalid
	}
	if err != nil {
		return err
	}
	if challenge.AttemptCount >= verificationMaxAttempts {
		return ErrVerificationLocked
	}
	if challenge.CodeHash != codeHash {
		_ = DB.Model(&EmailVerificationChallenge{}).
			Where("id = ?", challenge.ID).
			UpdateColumn("attempt_count", gorm.Expr("attempt_count + 1")).Error
		return ErrVerificationInvalid
	}
	res := DB.Model(&EmailVerificationChallenge{}).
		Where("id = ? AND consumed_at IS NULL AND expires_at > ? AND attempt_count < ?",
			challenge.ID, now, verificationMaxAttempts).
		Updates(map[string]any{"consumed_at": now})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrVerificationUsed
	}
	return nil
}

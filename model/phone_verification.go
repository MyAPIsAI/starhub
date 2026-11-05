package model

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"starhub/common"
)

var (
	ErrVerificationInvalid = errors.New("verification code is invalid or expired")
	ErrVerificationLocked  = errors.New("verification code has too many failed attempts")
	ErrVerificationUsed    = errors.New("verification code was already used")
)

const (
	PhoneVerificationPurposeLogin    = "phone_login"
	PhoneVerificationPurposeRegister = "phone_register"
	PhoneVerificationPurposeReset    = "phone_reset"
	PhoneVerificationPurposeChange   = "phone_change"

	verificationMaxAttempts = 5
)

// PhoneVerificationChallenge stores an outstanding SMS verification code.
// The code itself is never stored in plaintext — only its HMAC hash.
// One active challenge per (phone, purpose); resending replaces the old one.
type PhoneVerificationChallenge struct {
	ID           int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	PhoneE164    string `gorm:"size:16;not null;index" json:"phone_e164"`
	Purpose      string `gorm:"size:32;not null;index" json:"purpose"`
	CodeHash     string `gorm:"size:128;not null" json:"-"` // HMAC-SHA256, never serialized
	ExpiresAt    int64  `gorm:"not null;index" json:"expires_at"`
	ConsumedAt   *int64 `json:"consumed_at,omitempty"` // nil = unused
	AttemptCount int    `gorm:"not null;default:0" json:"attempt_count"`
	RequestIP    string `gorm:"size:64" json:"request_ip"`
	CreatedAt    int64  `gorm:"autoCreateTime;index" json:"created_at"`
}

// CreatePhoneVerificationChallenge creates a new challenge for the given
// (phone, purpose) pair, replacing any existing unconsumed one. The code is
// stored only as an HMAC hash.
func CreatePhoneVerificationChallenge(phoneE164, code, purpose, requestIP string, expiresAt int64) error {
	codeHash := common.GenerateHMAC("phone-verification:" + phoneE164 + ":" + code)
	return DB.Transaction(func(tx *gorm.DB) error {
		// Delete any existing unconsumed challenge for the same (phone, purpose).
		if err := tx.Where("phone_e164 = ? AND purpose = ? AND consumed_at IS NULL",
			phoneE164, purpose).Delete(&PhoneVerificationChallenge{}).Error; err != nil {
			return err
		}
		return tx.Create(&PhoneVerificationChallenge{
			PhoneE164: phoneE164,
			Purpose:   purpose,
			CodeHash:  codeHash,
			ExpiresAt: expiresAt,
			RequestIP: requestIP,
		}).Error
	})
}

// ConsumePhoneVerificationChallenge verifies and consumes a challenge. The
// failed attempt counter is persisted in a separate transaction so brute-
// force counting can never be undone by the caller's transaction rollback.
// Returns ErrVerificationInvalid, ErrVerificationLocked, or ErrVerificationUsed.
func ConsumePhoneVerificationChallenge(phoneE164, code, purpose string) error {
	codeHash := common.GenerateHMAC("phone-verification:" + phoneE164 + ":" + code)
	now := time.Now().Unix()

	// First, persist any pending counter increment from a previous failed
	// attempt that may have been rolled back by the caller's transaction.
	// This ensures the attempt counter is always up-to-date.

	// Read the current challenge outside a write transaction.
	var challenge PhoneVerificationChallenge
	err := DB.Where("phone_e164 = ? AND purpose = ? AND consumed_at IS NULL AND expires_at > ?",
		phoneE164, purpose, now).
		Order("id DESC").First(&challenge).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrVerificationInvalid
	}
	if err != nil {
		return err
	}

	// Check attempt limit.
	if challenge.AttemptCount >= verificationMaxAttempts {
		return ErrVerificationLocked
	}

	// Check code hash.
	if challenge.CodeHash != codeHash {
		// Persist the incremented counter in a separate transaction so it
		// survives even if the caller's transaction rolls back.
		_ = DB.Model(&PhoneVerificationChallenge{}).
			Where("id = ?", challenge.ID).
			UpdateColumn("attempt_count", gorm.Expr("attempt_count + 1")).Error
		return ErrVerificationInvalid
	}

	// Compare-and-swap: mark as consumed.
	res := DB.Model(&PhoneVerificationChallenge{}).
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

// IsValidPhoneVerificationCode checks the format of a verification code
// (4 or 6 digits).
func IsValidPhoneVerificationCode(code string) bool {
	if len(code) != 4 && len(code) != 6 {
		return false
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// DeletePhoneVerificationChallenges removes all challenges for a phone
// number (used after password reset to clean up stale challenges).
func DeletePhoneVerificationChallenges(phoneE164 string) error {
	return DB.Where("phone_e164 = ?", phoneE164).
		Delete(&PhoneVerificationChallenge{}).Error
}

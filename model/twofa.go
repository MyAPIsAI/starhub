package model

import (
	"time"

	"gorm.io/gorm"
	"starhub/common"
)

var (
	Err2FANotEnabled     = errString("2FA is not enabled for this user")
	Err2FAAlreadyOn      = errString("2FA is already enabled")
	Err2FALocked         = errString("2FA has too many failed attempts")
	Err2FAInvalidCode    = errString("invalid 2FA code")
	ErrBackupCodeUsed    = errString("backup code was already used")
	ErrBackupCodeInvalid = errString("invalid backup code")
)

func errString(s string) error { return &simpleErr{s} }

type simpleErr struct{ msg string }

func (e *simpleErr) Error() string { return e.msg }

const (
	twofaMaxAttempts  = 5
	twofaLockDuration = 5 * time.Minute
)

// TwoFA stores the TOTP secret and 2FA state for a user.
type TwoFA struct {
	ID             int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID         int64      `gorm:"uniqueIndex;not null" json:"user_id"`
	Secret         string     `gorm:"size:255;not null" json:"-"` // TOTP secret, never serialized
	IsEnabled      bool       `gorm:"not null;default:false" json:"is_enabled"`
	FailedAttempts int        `gorm:"not null;default:0" json:"-"`
	LockedUntil    *time.Time `json:"-"`
	CreatedAt      int64      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      int64      `gorm:"autoUpdateTime" json:"updated_at"`
}

// TwoFABackupCode stores a hashed backup code for a user's 2FA.
type TwoFABackupCode struct {
	ID        int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID    int64  `gorm:"index;not null" json:"user_id"`
	CodeHash  string `gorm:"size:256;not null" json:"-"` // bcrypt hash
	IsUsed    bool   `gorm:"not null;default:false" json:"is_used"`
	CreatedAt int64  `gorm:"autoCreateTime" json:"created_at"`
}

// GetTwoFA returns the 2FA state for a user, or Err2FANotEnabled.
func GetTwoFA(userID int64) (*TwoFA, error) {
	var t TwoFA
	err := DB.Where("user_id = ?", userID).First(&t).Error
	if err == gorm.ErrRecordNotFound {
		return nil, Err2FANotEnabled
	}
	return &t, err
}

// CreateTwoFASetup creates a pending (disabled) 2FA record with the given secret.
// If a record already exists (enabled or not), it is replaced.
func CreateTwoFASetup(userID int64, secret string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", userID).Delete(&TwoFA{}).Error; err != nil {
			return err
		}
		return tx.Create(&TwoFA{
			UserID:    userID,
			Secret:    secret,
			IsEnabled: false,
		}).Error
	})
}

// EnableTwoFA activates 2FA after verifying the first TOTP code. Generates
// backup codes in the same transaction.
func EnableTwoFA(userID int64, code string) ([]string, error) {
	var twofa TwoFA
	err := DB.Where("user_id = ?", userID).First(&twofa).Error
	if err == gorm.ErrRecordNotFound {
		return nil, Err2FANotEnabled
	}
	if err != nil {
		return nil, err
	}
	if twofa.IsEnabled {
		return nil, Err2FAAlreadyOn
	}
	if !common.ValidateTOTPCode(twofa.Secret, code) {
		return nil, Err2FAInvalidCode
	}
	codes, err := common.GenerateBackupCodes()
	if err != nil {
		return nil, err
	}
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&TwoFA{}).Where("id = ?", twofa.ID).
			Update("is_enabled", true).Error; err != nil {
			return err
		}
		// Delete old backup codes
		if err := tx.Where("user_id = ?", userID).Delete(&TwoFABackupCode{}).Error; err != nil {
			return err
		}
		for _, c := range codes {
			hashed, _ := common.HashPassword(c)
			if err := tx.Create(&TwoFABackupCode{
				UserID:   userID,
				CodeHash: hashed,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return codes, nil
}

// DisableTwoFA deactivates 2FA after verifying a TOTP code or backup code.
func DisableTwoFA(userID int64, code string) error {
	twofa, err := GetTwoFA(userID)
	if err != nil {
		return err
	}
	if !twofa.IsEnabled {
		return Err2FANotEnabled
	}
	// Check if it's a TOTP code or a backup code.
	if common.ValidateTOTPCode(twofa.Secret, code) {
		return DB.Model(&TwoFA{}).Where("user_id = ?", userID).
			Update("is_enabled", false).Error
	}
	// Try backup code.
	return consumeBackupCode(userID, code)
}

func consumeBackupCode(userID int64, code string) error {
	var codes []TwoFABackupCode
	if err := DB.Where("user_id = ? AND is_used = false", userID).Find(&codes).Error; err != nil {
		return err
	}
	for i := range codes {
		if common.VerifyPassword(code, codes[i].CodeHash) {
			return DB.Model(&TwoFABackupCode{}).Where("id = ?", codes[i].ID).
				Update("is_used", true).Error
		}
	}
	return ErrBackupCodeInvalid
}

// VerifyTwoFA validates a TOTP code (not backup codes) for login.
// Increments failed attempts on wrong code; locks after maxAttempts.
func VerifyTwoFA(userID int64, code string) error {
	twofa, err := GetTwoFA(userID)
	if err != nil {
		return err
	}
	if !twofa.IsEnabled {
		return Err2FANotEnabled
	}
	if twofa.LockedUntil != nil && time.Now().Before(*twofa.LockedUntil) {
		return Err2FALocked
	}
	if common.ValidateTOTPCode(twofa.Secret, code) {
		// Reset failed attempts on success.
		_ = DB.Model(&TwoFA{}).Where("id = ?", twofa.ID).
			Updates(map[string]any{"failed_attempts": 0, "locked_until": nil}).Error
		return nil
	}
	// Increment failed attempts.
	attempts := twofa.FailedAttempts + 1
	updates := map[string]any{"failed_attempts": attempts}
	if attempts >= twofaMaxAttempts {
		lockedUntil := time.Now().Add(twofaLockDuration)
		updates["locked_until"] = lockedUntil
	}
	_ = DB.Model(&TwoFA{}).Where("id = ?", twofa.ID).Updates(updates).Error
	return Err2FAInvalidCode
}

// RegenerateBackupCodes generates new backup codes, invalidating all old ones.
func RegenerateBackupCodes(userID int64, code string) ([]string, error) {
	twofa, err := GetTwoFA(userID)
	if err != nil {
		return nil, err
	}
	if !twofa.IsEnabled {
		return nil, Err2FANotEnabled
	}
	if !common.ValidateTOTPCode(twofa.Secret, code) {
		return nil, Err2FAInvalidCode
	}
	codes, err := common.GenerateBackupCodes()
	if err != nil {
		return nil, err
	}
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", userID).Delete(&TwoFABackupCode{}).Error; err != nil {
			return err
		}
		for _, c := range codes {
			hashed, _ := common.HashPassword(c)
			if err := tx.Create(&TwoFABackupCode{
				UserID:   userID,
				CodeHash: hashed,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return codes, nil
}

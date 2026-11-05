package model

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"gorm.io/gorm"
)

var (
	ErrPhoneAlreadyBound = errors.New("phone number is already bound to another user")
	ErrUserPhoneNotFound = errors.New("no phone identity found for this user")
	ErrPhoneFormat       = errors.New("invalid phone number format")
)

// UserPhoneIdentity binds a user to a verified phone number in E.164 format.
// One user has at most one phone (uniqueIndex on user_id); one phone is bound
// to at most one user (uniqueIndex on phone_e164).
type UserPhoneIdentity struct {
	ID         int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID     int64  `gorm:"uniqueIndex;not null" json:"user_id"`
	PhoneE164  string `gorm:"size:16;uniqueIndex;not null" json:"phone_e164"`
	VerifiedAt int64  `gorm:"not null" json:"verified_at"`
	CreatedAt  int64  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt  int64  `gorm:"autoUpdateTime" json:"updated_at"`
}

// chinaMobilePattern matches mainland China mobile numbers: 11 digits,
// starting with 1 and second digit 3-9.
var chinaMobilePattern = regexp.MustCompile(`^1[3-9]\d{9}$`)

// NormalizePhone validates and normalizes a phone number to E.164 format.
// Currently supports mainland China numbers only (+86 prefix). Returns
// ErrPhoneFormat for invalid input.
func NormalizePhone(value string) (string, error) {
	phone := strings.TrimSpace(value)
	if phone == "" {
		return "", ErrPhoneFormat
	}
	// Strip common separators.
	phone = strings.NewReplacer(" ", "", "-", "", "(", "", ")", "").Replace(phone)
	// Normalize to bare national number.
	switch {
	case strings.HasPrefix(phone, "+86"):
		phone = phone[3:]
	case strings.HasPrefix(phone, "0086"):
		phone = phone[4:]
	case strings.HasPrefix(phone, "86") && len(phone) == 13:
		phone = phone[2:]
	}
	if !chinaMobilePattern.MatchString(phone) {
		return "", fmt.Errorf("%w: must be a valid mainland China mobile number", ErrPhoneFormat)
	}
	return "+86" + phone, nil
}

// CreateUserPhoneIdentity inserts a new phone binding. Returns
// ErrPhoneAlreadyBound if the phone is already bound to another user.
func CreateUserPhoneIdentity(tx *gorm.DB, userID int64, phoneE164 string, verifiedAt int64) error {
	if tx == nil {
		tx = DB
	}
	// Check whether the phone is already bound to a different user.
	var existing UserPhoneIdentity
	err := tx.Where("phone_e164 = ?", phoneE164).First(&existing).Error
	if err == nil {
		if existing.UserID != userID {
			return ErrPhoneAlreadyBound
		}
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return tx.Create(&UserPhoneIdentity{
		UserID:     userID,
		PhoneE164:  phoneE164,
		VerifiedAt: verifiedAt,
	}).Error
}

// GetUserByPhone returns the user bound to the given E.164 phone number, or
// ErrUserNotFound when no match exists.
func GetUserByPhone(phoneE164 string) (*User, error) {
	var identity UserPhoneIdentity
	if err := DB.Where("phone_e164 = ?", phoneE164).First(&identity).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return GetUserByID(identity.UserID)
}

// GetUserPhone returns the phone identity for a user, or ErrUserPhoneNotFound.
func GetUserPhone(userID int64) (*UserPhoneIdentity, error) {
	var identity UserPhoneIdentity
	err := DB.Where("user_id = ?", userID).First(&identity).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrUserPhoneNotFound
	}
	if err != nil {
		return nil, err
	}
	return &identity, nil
}

// DeleteUserPhone removes the phone binding for a user (used during account
// deactivation or phone change).
func DeleteUserPhone(tx *gorm.DB, userID int64) error {
	if tx == nil {
		tx = DB
	}
	return tx.Where("user_id = ?", userID).Delete(&UserPhoneIdentity{}).Error
}

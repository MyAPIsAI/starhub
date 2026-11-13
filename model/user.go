package model

import (
	"errors"
	"math/big"

	"gorm.io/gorm"

	"starhub/common"
)

var ErrUserNotFound = errors.New("user not found")

type User struct {
	QuotaRemainderNano     int64  `gorm:"not null;default:0" json:"-"`
	UsedQuotaRemainderNano int64  `gorm:"not null;default:0" json:"-"`
	ID                     int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	Username               string `gorm:"uniqueIndex;size:64;not null" json:"username"`
	Email                  string `gorm:"size:64;not null;default:''" json:"email"`
	Password               string `gorm:"size:256;not null;default:''" json:"-"`
	Role                   int    `gorm:"not null;default:1" json:"role"`
	Status                 int    `gorm:"not null;default:1" json:"status"`
	Quota                  int64  `gorm:"not null;default:0" json:"quota"`
	UsedQuota              int64  `gorm:"not null;default:0" json:"used_quota"`
	Group                  string `gorm:"column:group;size:64;not null;default:'default'" json:"group"`
	// AffCode is the user's invite code (others register with it to become
	// their invitees). Auto-generated on first read when empty.
	AffCode string `gorm:"uniqueIndex;size:32;not null;default:''" json:"aff_code"`
	// InviterID is the user who invited this user (0 = no inviter).
	InviterID int64 `gorm:"index;not null;default:0" json:"inviter_id"`
	// Language is the user's preferred UI/notification language (zh, en, ar, ko).
	// Empty means no preference; the system falls back to Accept-Language or default.
	Language            string `gorm:"size:8;not null;default:''" json:"language"`
	CreatedAt           int64  `gorm:"autoCreateTime" json:"created_at"`
	DeactivationVersion int64  `gorm:"not null;default:0" json:"-"`
}

func (u *User) IsAdmin() bool {
	return common.IsAdminRole(u.Role)
}

func GetUserByID(id int64) (*User, error) {
	if id <= 0 {
		return nil, ErrUserNotFound
	}
	var u User
	err := DB.First(&u, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func GetUserByUsername(username string) (*User, error) {
	var u User
	err := DB.Where("username = ?", username).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func CreateUser(u *User) error {
	return DB.Create(u).Error
}

func ListUsers() ([]User, error) {
	var users []User
	err := DB.Order("id ASC").Find(&users).Error
	return users, err
}

func UpdateUserFields(id int64, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	if _, changed := fields["quota"]; changed {
		user, err := GetUserByID(id)
		if err != nil {
			return err
		}
		expected := user.Quota
		return UpdateUserFieldsWithQuota(id, fields, &expected)
	}
	return DB.Model(&User{}).Where("id = ?", id).Updates(fields).Error
}

func DecreaseUserQuota(id int64, amount int64) (bool, error) {
	if amount <= 0 {
		return true, nil
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		_, err := applyUserMoneyDelta(tx, id, new(big.Int).Neg(common.QuotaMoney(amount, 0)), "legacy_debit", legacyMoneyOperation("legacy_debit", "", amount), "Legacy API quota debit", "system_quota", 0)
		return err
	})
	if errors.Is(err, ErrUserQuotaShort) {
		return false, nil
	}
	return err == nil, err
}

func IncreaseUserQuota(id int64, amount int64) error {
	if amount <= 0 {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		_, err := applyUserMoneyDelta(tx, id, common.QuotaMoney(amount, 0), "system_credit", legacyMoneyOperation("system_credit", "", amount), "Authorized system quota credit", "system_quota", 0)
		return err
	})
}

func IncreaseUserUsedQuota(id int64, amount int64) error {
	if amount <= 0 {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		user, account, err := lockMoneyUser(tx, id)
		if err != nil {
			return err
		}
		delta := common.QuotaMoney(amount, 0)
		used := common.QuotaMoney(user.UsedQuota, user.UsedQuotaRemainderNano)
		used.Add(used, delta)
		if err := setMoneyPair(tx, &User{}, id, "used_quota", used); err != nil {
			return err
		}
		_, err = recordMoneyMovement(tx, account, moneyMovement{Spent: delta, Kind: "counter_import", OperationID: legacyMoneyOperation("counter_import", "", amount), Reason: "Legacy usage counter import", FundingSource: "legacy_counter_import"})
		return err
	})
}

// GetUserByAffCode returns the user who owns the invite code, or
// ErrUserNotFound when the code does not match any user.
func GetUserByAffCode(code string) (*User, error) {
	if code == "" {
		return nil, ErrUserNotFound
	}
	var u User
	err := DB.Where("aff_code = ?", code).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

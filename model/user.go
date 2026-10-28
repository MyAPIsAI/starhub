package model

import (
	"errors"

	"gorm.io/gorm"

	"starhub/common"
)

var ErrUserNotFound = errors.New("user not found")

type User struct {
	ID        int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	Username  string `gorm:"uniqueIndex;size:64;not null" json:"username"`
	Email     string `gorm:"size:64;not null;default:''" json:"email"`
	Password  string `gorm:"size:256;not null;default:''" json:"-"`
	Role      int    `gorm:"not null;default:1" json:"role"`
	Status    int    `gorm:"not null;default:1" json:"status"`
	Quota     int64  `gorm:"not null;default:0" json:"quota"`
	UsedQuota int64  `gorm:"not null;default:0" json:"used_quota"`
	Group     string `gorm:"column:group;size:64;not null;default:'default'" json:"group"`
	// AffCode is the user's invite code (others register with it to become
	// their invitees). Auto-generated on first read when empty.
	AffCode string `gorm:"uniqueIndex;size:32;not null;default:''" json:"aff_code"`
	// InviterID is the user who invited this user (0 = no inviter).
	InviterID int64 `gorm:"index;not null;default:0" json:"inviter_id"`
	CreatedAt int64 `gorm:"autoCreateTime" json:"created_at"`
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
	return DB.Model(&User{}).Where("id = ?", id).Updates(fields).Error
}

func DecreaseUserQuota(id int64, amount int64) (bool, error) {
	if amount <= 0 {
		return true, nil
	}
	res := DB.Model(&User{}).Where("id = ? AND quota >= ?", id, amount).
		Updates(map[string]any{"quota": gorm.Expr("quota - ?", amount)})
	return res.RowsAffected > 0, res.Error
}

func IncreaseUserQuota(id int64, amount int64) error {
	if amount <= 0 {
		return nil
	}
	return DB.Model(&User{}).Where("id = ?", id).
		Updates(map[string]any{"quota": gorm.Expr("quota + ?", amount)}).Error
}

func IncreaseUserUsedQuota(id int64, amount int64) error {
	if amount <= 0 {
		return nil
	}
	return DB.Model(&User{}).Where("id = ?", id).
		Updates(map[string]any{"used_quota": gorm.Expr("used_quota + ?", amount)}).Error
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

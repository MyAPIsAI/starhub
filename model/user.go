package model

import (
	"errors"

	"gorm.io/gorm"

	"starhub/common"
)

var ErrUserNotFound = errors.New("user not found")

// Every staff role has a personal workspace with owner-scoped permissions.
func (u *User) HasPlatformWorkspace() bool {
	return u.AccountKind == "staff" && common.IsStaffRole(u.Role)
}

type User struct {
	PreferredLanguage  string `gorm:"size:8;not null;default:''" json:"preferred_language"`
	ReservedQuota      int64  `gorm:"not null;default:0" json:"-"`
	ControlRevision    int64  `gorm:"not null;default:0" json:"-"`
	Phone              string `gorm:"size:32;not null;default:''" json:"-"`
	ConcurrencyLimit   int    `gorm:"not null;default:50" json:"concurrency_limit"`
	QuotaNanoRemainder int64  `gorm:"not null;default:0" json:"quota_nano_remainder"`
	AccountKind        string `gorm:"not null;default:'customer';index;uniqueIndex:idx_users_realm_username" json:"account_kind"`
	MustChangePassword bool   `gorm:"not null;default:false" json:"must_change_password"`
	SecurityVersion    int64  `gorm:"not null;default:0" json:"-"`
	TOTPSecret         string `gorm:"type:text;not null;default:''" json:"-"`
	TOTPLastStep       int64  `gorm:"not null;default:0" json:"-"`
	OTPFailures        int    `gorm:"not null;default:0" json:"-"`
	OTPLockedUntil     int64  `gorm:"not null;default:0" json:"-"`
	ID                 int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	Username           string `gorm:"uniqueIndex:idx_users_realm_username;size:64;not null" json:"username"`
	Email              string `gorm:"size:64;not null;default:''" json:"email"`
	Password           string `gorm:"size:256;not null;default:''" json:"-"`
	Role               int    `gorm:"not null;default:1" json:"role"`
	Status             int    `gorm:"not null;default:1" json:"status"`
	Quota              int64  `gorm:"not null;default:0" json:"quota"`
	UsedQuota          int64  `gorm:"not null;default:0" json:"used_quota"`
	Group              string `gorm:"column:group;size:64;not null;default:'default'" json:"group"`
	// AffCode is the user's invite code (others register with it to become
	// their invitees). Auto-generated on first read when empty.
	AffCode string `gorm:"uniqueIndex;size:32;not null;default:''" json:"aff_code"`
	// InviterID is the user who invited this user (0 = no inviter).
	InviterID           int64 `gorm:"index;not null;default:0" json:"inviter_id"`
	CreatedAt           int64 `gorm:"autoCreateTime" json:"created_at"`
	DeactivationVersion int64 `gorm:"not null;default:0" json:"-"`
}

func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.AccountKind == "" {
		u.AccountKind = "customer"
		if common.IsStaffRole(u.Role) {
			u.AccountKind = "staff"
		}
	}
	if u.AccountKind == "staff" {
		if !common.IsStaffRole(u.Role) {
			return errors.New("invalid staff role")
		}
		if u.Password != "" || u.TOTPSecret != "" {
			return errors.New("staff credentials must be stored in staff_accounts")
		}
	} else if u.AccountKind != "customer" || common.IsStaffRole(u.Role) {
		return errors.New("invalid customer account role")
	}
	return nil
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
	if u.AccountKind == "staff" {
		return staffUser(DB, u.ID)
	}
	return &u, nil
}

func GetUserByUsername(username string) (*User, error) {
	var u User
	err := DB.Where("username = ? AND account_kind = ?", username, "customer").First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func CreateUser(u *User) error {
	if u.AccountKind == "staff" || (u.AccountKind == "" && common.IsStaffRole(u.Role)) {
		return DB.Transaction(func(tx *gorm.DB) error { return createStaffIdentity(tx, u) })
	}
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
	var owner User
	if err := DB.First(&owner, id).Error; err != nil {
		return err
	}
	if owner.AccountKind == "staff" {
		for _, field := range []string{"username", "password", "account_kind", "role", "status", "must_change_password", "security_version", "totp_secret", "totp_last_step", "otp_failures", "otp_locked_until"} {
			if _, exists := fields[field]; exists {
				return errors.New("staff identity changes require staff account management")
			}
		}
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

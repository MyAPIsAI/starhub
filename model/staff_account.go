package model

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"starhub/common"
)

// StaffAccount is the sole authority for internal login credentials and roles.
// ID is also the immutable workspace owner ID in users. That row is not a
// customer login and retains existing keys, balances and accounting references.
type StaffAccount struct {
	ID                 int64  `gorm:"primaryKey;autoIncrement:false"`
	Username           string `gorm:"uniqueIndex;size:64;not null"`
	Password           string `gorm:"size:256;not null" json:"-"`
	Role               int    `gorm:"not null"`
	Status             int    `gorm:"not null"`
	MustChangePassword bool   `gorm:"not null;default:false"`
	SecurityVersion    int64  `gorm:"not null;default:0"`
	TOTPSecret         string `gorm:"type:text;not null;default:''" json:"-"`
	TOTPLastStep       int64  `gorm:"not null;default:0" json:"-"`
	OTPFailures        int    `gorm:"not null;default:0" json:"-"`
	OTPLockedUntil     int64  `gorm:"not null;default:0" json:"-"`
	CreatedAt          int64  `gorm:"autoCreateTime"`
}

func staffAccountFromUser(u *User) StaffAccount {
	return StaffAccount{ID: u.ID, Username: u.Username, Password: u.Password,
		Role: u.Role, Status: u.Status, MustChangePassword: u.MustChangePassword,
		SecurityVersion: u.SecurityVersion, TOTPSecret: u.TOTPSecret,
		TOTPLastStep: u.TOTPLastStep, OTPFailures: u.OTPFailures,
		OTPLockedUntil: u.OTPLockedUntil, CreatedAt: u.CreatedAt}
}

func (s *StaffAccount) apply(u *User) {
	u.Username, u.Password, u.Role, u.Status = s.Username, s.Password, s.Role, s.Status
	u.MustChangePassword, u.SecurityVersion = s.MustChangePassword, s.SecurityVersion
	u.TOTPSecret, u.TOTPLastStep = s.TOTPSecret, s.TOTPLastStep
	u.OTPFailures, u.OTPLockedUntil = s.OTPFailures, s.OTPLockedUntil
}

func staffUser(tx *gorm.DB, id int64) (*User, error) {
	var account StaffAccount
	if err := tx.First(&account, id).Error; err != nil {
		return nil, err
	}
	var owner User
	if err := tx.Where("id = ? AND account_kind = ?", id, "staff").First(&owner).Error; err != nil {
		return nil, err
	}
	account.apply(&owner)
	return &owner, nil
}

func GetStaffByID(id int64) (*User, error) {
	if id <= 0 {
		return nil, ErrUserNotFound
	}
	return staffUser(DB, id)
}

func GetStaffByUsername(username string) (*User, error) {
	var account StaffAccount
	if err := DB.Where("username = ?", username).First(&account).Error; err != nil {
		return nil, err
	}
	return staffUser(DB, account.ID)
}

func ListStaffAccounts() ([]User, error) {
	var accounts []StaffAccount
	if err := DB.Order("id ASC").Find(&accounts).Error; err != nil {
		return nil, err
	}
	users := make([]User, 0, len(accounts))
	for _, account := range accounts {
		users = append(users, User{ID: account.ID, AccountKind: "staff", CreatedAt: account.CreatedAt})
		account.apply(&users[len(users)-1])
	}
	return users, nil
}

func createStaffIdentity(tx *gorm.DB, user *User) error {
	if !common.IsStaffRole(user.Role) {
		return errors.New("invalid staff role")
	}
	user.AccountKind, user.MustChangePassword = "staff", true
	account := staffAccountFromUser(user)
	owner := *user
	owner.Password, owner.TOTPSecret = "", ""
	owner.SecurityVersion, owner.TOTPLastStep, owner.OTPFailures, owner.OTPLockedUntil = 0, 0, 0, 0
	owner.MustChangePassword = false
	if err := tx.Create(&owner).Error; err != nil {
		return err
	}
	account.ID, account.CreatedAt = owner.ID, owner.CreatedAt
	account.Status = owner.Status
	if err := tx.Create(&account).Error; err != nil {
		return err
	}
	*user = owner
	account.apply(user)
	return nil
}

// Migrate and scrub in one transaction. Existing identity rows win on reruns;
// an old users row must never restore a rotated password or reset OTP counters.
func migrateStaffIdentities(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if tx.Migrator().HasIndex(&User{}, "idx_users_username") {
			if err := tx.Migrator().DropIndex(&User{}, "idx_users_username"); err != nil {
				return err
			}
		}
		var legacy []User
		if err := tx.Where("account_kind = ?", "staff").Find(&legacy).Error; err != nil {
			return err
		}
		for _, user := range legacy {
			account := staffAccountFromUser(&user)
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoNothing: true}).Create(&account).Error; err != nil {
				return err
			}
		}
		return tx.Model(&User{}).Where("account_kind = ?", "staff").Updates(map[string]any{
			"password": "", "totp_secret": "", "totp_last_step": 0, "otp_failures": 0,
			"otp_locked_until": 0, "must_change_password": false, "security_version": 0,
		}).Error
	})
}

package model

import (
	"errors"
	"regexp"
	"strings"
	"unicode"

	"gorm.io/gorm"
	"starhub/common"
)

// OperateStaffAccount is restricted to the local operator CLI, not HTTP routes.
// Database access is the operator trust boundary; the OS identity and reason
// are committed atomically with initialization or factor reset.
func OperateStaffAccount(action, username, password, operator, reason string) error {
	if strings.TrimSpace(operator) == "" || strings.TrimSpace(reason) == "" || len(operator) > 64 || len(reason) > 512 {
		return errors.New("operator identity and a reason of at most 512 bytes are required")
	}
	if action != "initialize" && action != "reset-2fa" {
		return errors.New("unsupported operator action")
	}
	var hashed string
	if action == "initialize" {
		switch strings.ToLower(password) {
		case "password1", "abc12345", "qwerty12", "12345678", "11111111", "00000000", "abcd1234", "test1234", "admin123", "letmein1", "welcome1", "monkey12":
			return errors.New("choose a less common password")
		}
		letter, digit := false, false
		for _, c := range password {
			letter = letter || unicode.IsLetter(c)
			digit = digit || unicode.IsDigit(c)
		}
		if !regexp.MustCompile(`^[A-Za-z0-9_.-]{2,24}$`).MatchString(username) || len(password) < 8 || len(password) > 64 || !letter || !digit {
			return errors.New("a valid username and a strong 8-64 byte password are required")
		}
		var err error
		hashed, err = common.HashPassword(password)
		if err != nil {
			return err
		}
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("UPDATE users SET id = id WHERE role = ?", common.RoleRoot).Error; err != nil {
			return err
		}
		var user User
		if action == "initialize" {
			var count int64
			if err := tx.Model(&User{}).Where("role = ?", common.RoleRoot).Count(&count).Error; err != nil {
				return err
			}
			if count != 0 {
				return errors.New("a root account already exists")
			}
			user = User{Username: username, Password: hashed, Role: common.RoleRoot, Status: common.StatusEnabled, Group: common.DefaultGroup, AffCode: common.GenerateAffCode()}
			if err := tx.Create(&user).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Where("username = ?", username).First(&user).Error; err != nil {
				return err
			}
			if !common.IsStaffRole(user.Role) {
				return errors.New("only staff factors may be reset by this command")
			}
			if err := tx.Where("user_id = ?", user.ID).Delete(&TwoFA{}).Error; err != nil {
				return err
			}
			if err := tx.Where("user_id = ?", user.ID).Delete(&TwoFABackupCode{}).Error; err != nil {
				return err
			}
			if err := tx.Model(&User{}).Where("id = ?", user.ID).UpdateColumn("deactivation_version", gorm.Expr("deactivation_version + 1")).Error; err != nil {
				return err
			}
		}
		return tx.Create(&AuditLog{UserID: user.ID, Username: operator, Method: "CLI", Path: "/operator/" + action, Status: 200, Reason: reason}).Error
	})
}

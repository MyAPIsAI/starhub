package model

import (
	"errors"
	"starhub/common"
)

var ErrUserProfileConflict = errors.New("account profile changed")

// UpdateOwnedProfile compares the security-relevant snapshot in the same SQL
// statement that writes one field. Password hashing happens before this call.
func UpdateOwnedProfile(expected *User, field, value string) error {
	if expected == nil || expected.ID <= 0 || expected.Status != common.StatusEnabled || (field != "password" && field != "email") {
		return errors.New("invalid account profile update")
	}
	result := DB.Model(&User{}).Where("id = ? AND status = ? AND password = ? AND email = ? AND role = ? AND \"group\" = ?", expected.ID, expected.Status, expected.Password, expected.Email, expected.Role, expected.Group).Update(field, value)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrUserProfileConflict
	}
	return nil
}

package model

import (
	"errors"

	"starhub/common"
)

var ErrUserRoleConflict = errors.New("user role changed; reload before updating")
var ErrInvalidFinanceRole = errors.New("finance access can only change regular user or finance roles")

// SetUserFinanceRole atomically changes only the expected non-admin role.
// It cannot promote, demote or overwrite an administrator or Root account.
func SetUserFinanceRole(id int64, enabled bool, expectedRole int) error {
	if expectedRole != common.RoleUser && expectedRole != common.RoleFinance {
		return ErrInvalidFinanceRole
	}
	role := common.RoleUser
	if enabled {
		role = common.RoleFinance
	}
	result := DB.Model(&User{}).Where("id = ? AND role = ?", id, expectedRole).Update("role", role)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		return nil
	}
	if _, err := GetUserByID(id); err != nil {
		return err
	}
	return ErrUserRoleConflict
}

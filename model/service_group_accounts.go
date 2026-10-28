package model

import (
	"errors"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

var ErrInvalidServiceGroupAccounts = errors.New("custom account_ids must contain 1-100 distinct existing canonical server user IDs, within 1024 bytes")

// Called under the configuration writer lock. Unchanged historical lists can
// be retained while editing other fields; replacing them requires real users.
func validateServiceGroupAccounts(tx *gorm.DB, group *ServiceGroupConfig) error {
	if group.Visibility != "custom" {
		return nil
	}
	var current ServiceGroupConfig
	err := tx.Select("visibility", "account_ids").Where("group_id = ?", strings.TrimSpace(group.GroupID)).First(&current).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err == nil && current.Visibility == "custom" && current.AccountIDs == group.AccountIDs {
		return nil
	}
	parts := strings.Split(group.AccountIDs, ",")
	if len(parts) > 100 || len(group.AccountIDs) > 1024 {
		return ErrInvalidServiceGroupAccounts
	}
	ids := make([]int64, 0, len(parts))
	seen := make(map[int64]bool, len(parts))
	for _, value := range parts {
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil || id <= 0 || strconv.FormatInt(id, 10) != value || seen[id] {
			return ErrInvalidServiceGroupAccounts
		}
		seen[id] = true
		ids = append(ids, id)
	}
	var count int64
	if err := tx.Model(&User{}).Where("id IN ?", ids).Count(&count).Error; err != nil {
		return err
	}
	if count != int64(len(ids)) {
		return ErrInvalidServiceGroupAccounts
	}
	return nil
}

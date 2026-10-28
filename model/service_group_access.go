package model

import (
	"context"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

// ServiceGroupDeletion prevents stale process-local ratios from reopening a
// deleted definition as an unrestricted legacy group. Explicit recreation clears it.
type ServiceGroupDeletion struct {
	GroupID   string `gorm:"primaryKey;size:64"`
	CreatedAt int64  `gorm:"autoCreateTime"`
}

// ServiceGroupAccess resolves metadata restrictions in one read. Missing rows
// retain legacy access; callers must separately require a configured live ratio.
// Custom lists contain exact decimal server user IDs, never Demo identifiers.
func ServiceGroupAccess(ctx context.Context, groups []string, userID int64) (map[string]bool, error) {
	allowed := make(map[string]bool, len(groups))
	for _, group := range groups {
		if group != "" {
			allowed[group] = true
		}
	}
	if len(allowed) == 0 {
		return allowed, nil
	}
	keys := make([]string, 0, len(allowed))
	for group := range allowed {
		keys = append(keys, group)
	}
	var rows []ServiceGroupConfig
	var deleted []ServiceGroupDeletion
	// Both reads share a snapshot so recreation cannot expose a transient
	// missing row without its deletion marker and grant legacy access.
	if err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Select("group_id", "enabled", "visibility", "account_ids").
			Where("group_id IN ?", keys).Find(&rows).Error; err != nil {
			return err
		}
		return tx.Select("group_id").Where("group_id IN ?", keys).Find(&deleted).Error
	}); err != nil {
		return nil, err
	}
	identity := strconv.FormatInt(userID, 10)
	for _, row := range rows {
		admitted := row.Enabled && row.Visibility == "common"
		if row.Enabled && row.Visibility == "custom" && userID > 0 {
			for _, account := range strings.Split(row.AccountIDs, ",") {
				if strings.TrimSpace(account) == identity {
					admitted = true
					break
				}
			}
		}
		allowed[row.GroupID] = admitted
	}
	for _, row := range deleted {
		allowed[row.GroupID] = false
	}
	return allowed, nil
}

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

// ServiceGroupPermission separates unavailable definitions from account denials.
type ServiceGroupPermission struct {
	Allowed     bool
	Unavailable bool
}

// ServiceGroupAccess resolves metadata restrictions in one read. Missing rows
// retain legacy access; callers must separately require a configured live ratio.
// Custom lists contain exact decimal server user IDs, never Demo identifiers.
func ServiceGroupAccess(ctx context.Context, groups []string, userID int64) (map[string]bool, error) {
	states, err := ServiceGroupAccessDetails(ctx, groups, userID)
	if err != nil {
		return nil, err
	}
	allowed := make(map[string]bool, len(states))
	for group, state := range states {
		allowed[group] = state.Allowed
	}
	return allowed, nil
}

// ServiceGroupAccessDetails distinguishes unavailable groups from denied accounts
// using one database snapshot, including when process-local ratios are stale.
func ServiceGroupAccessDetails(ctx context.Context, groups []string, userID int64) (map[string]ServiceGroupPermission, error) {
	allowed := make(map[string]ServiceGroupPermission, len(groups))
	for _, group := range groups {
		if group != "" {
			allowed[group] = ServiceGroupPermission{Allowed: true}
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
		allowed[row.GroupID] = ServiceGroupPermission{Allowed: admitted, Unavailable: !row.Enabled}
	}
	for _, row := range deleted {
		allowed[row.GroupID] = ServiceGroupPermission{Unavailable: true}
	}
	return allowed, nil
}

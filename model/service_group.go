package model

import (
	"errors"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrServiceGroupNotFound = errors.New("service group not found")

// ServiceGroupConfig stores metadata for a service group that the
// plain group_ratio option map cannot hold: display name, description,
// enabled flag, visibility scope, and account whitelist. The multiplier
// (billing ratio) remains in the group_ratio option; this table is
// synced alongside it by the controller.
type ServiceGroupConfig struct {
	ID            int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	GroupID       string `gorm:"uniqueIndex;size:64;not null" json:"group_id"`
	Name          string `gorm:"size:128;not null;default:''" json:"name"`
	Description   string `gorm:"size:512;not null;default:''" json:"description"`
	Enabled       bool   `gorm:"not null;default:true" json:"enabled"`
	Visibility    string `gorm:"size:16;not null;default:'common'" json:"visibility"` // common | custom
	AccountIDs    string `gorm:"size:1024;not null;default:''" json:"account_ids"`    // comma-separated
	Multiplier    string `gorm:"size:32;not null;default:'1'" json:"multiplier"`      // decimal string, synced to group_ratio
	Revision      int64  `gorm:"not null;default:0" json:"revision"`
	SLASuccessPPM *int64 `json:"-"`
	SLAP95MS      *int64 `json:"-"`
	SLARevision   int64  `gorm:"not null;default:0" json:"-"`
	SLAUpdatedAt  int64  `gorm:"not null;default:0" json:"-"`
	CreatedAt     int64  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     int64  `gorm:"autoUpdateTime" json:"updated_at"`
}

// UpsertServiceGroup creates or updates a service group config row.
func UpsertServiceGroup(sgc *ServiceGroupConfig) error {
	if sgc == nil {
		return errors.New("service group is required")
	}
	stored := *sgc
	if err := DB.Transaction(func(tx *gorm.DB) error { return upsertServiceGroup(tx, &stored) }); err != nil {
		return err
	}
	*sgc = stored
	return nil
}

func upsertServiceGroup(db *gorm.DB, sgc *ServiceGroupConfig) error {
	sgc.GroupID = strings.TrimSpace(sgc.GroupID)
	if sgc.GroupID == "" {
		return errors.New("group_id is required")
	}
	if sgc.Visibility == "" {
		sgc.Visibility = "common"
	}
	if sgc.Multiplier == "" {
		sgc.Multiplier = "1"
	}
	// Try update first; if no row matches, create.
	res := db.Model(&ServiceGroupConfig{}).
		Where("group_id = ? AND revision >= 0 AND revision < ?", sgc.GroupID, maxServiceGroupRevision).
		Updates(map[string]any{
			"name":        sgc.Name,
			"description": sgc.Description,
			"enabled":     sgc.Enabled,
			"visibility":  sgc.Visibility,
			"account_ids": sgc.AccountIDs,
			"multiplier":  sgc.Multiplier,
			"revision":    gorm.Expr("revision + 1"),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected > 0 {
		return reloadServiceGroup(db, sgc)
	}
	// On the create path, use a map so zero-value fields (Enabled=false)
	// are persisted — GORM's struct Create skips zero values.
	if err := db.Model(&ServiceGroupConfig{}).Create(map[string]any{
		"group_id":    sgc.GroupID,
		"name":        sgc.Name,
		"description": sgc.Description,
		"enabled":     sgc.Enabled,
		"visibility":  sgc.Visibility,
		"account_ids": sgc.AccountIDs,
		"multiplier":  sgc.Multiplier,
		"revision":    int64(1),
	}).Error; err != nil {
		return err
	}
	return reloadServiceGroup(db, sgc)
}

func reloadServiceGroup(db *gorm.DB, sgc *ServiceGroupConfig) error {
	var stored ServiceGroupConfig
	if err := db.Where("group_id = ?", sgc.GroupID).First(&stored).Error; err != nil {
		return err
	}
	if err := db.Where("group_id = ?", sgc.GroupID).Delete(&ServiceGroupDeletion{}).Error; err != nil {
		return err
	}
	*sgc = stored
	return nil
}

// GetServiceGroup returns a service group config by its group_id.
func GetServiceGroup(groupID string) (*ServiceGroupConfig, error) {
	var sgc ServiceGroupConfig
	err := DB.Where("group_id = ?", groupID).First(&sgc).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrServiceGroupNotFound
	}
	if err != nil {
		return nil, err
	}
	return &sgc, nil
}

// ListServiceGroups returns all service group configs, ordered by group_id.
func ListServiceGroups() ([]ServiceGroupConfig, error) {
	var groups []ServiceGroupConfig
	err := DB.Order("group_id ASC").Find(&groups).Error
	return groups, err
}

// DeleteServiceGroup removes a service group config by group_id.
func DeleteServiceGroup(groupID string) error {
	return DB.Transaction(func(tx *gorm.DB) error { return deleteServiceGroup(tx, groupID) })
}

func deleteServiceGroup(db *gorm.DB, groupID string) error {
	res := db.Where("group_id = ?", groupID).Delete(&ServiceGroupConfig{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrServiceGroupNotFound
	}
	return db.Clauses(clause.OnConflict{DoNothing: true}).Create(&ServiceGroupDeletion{GroupID: groupID}).Error
}

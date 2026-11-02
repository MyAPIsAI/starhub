package model

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
	"starhub/setting"
)

// Default groups are bootstrap data for a pristine installation, never a
// recurring migration that restores deleted groups or overwrites operator edits.
func seedDefaultServiceGroups(tx *gorm.DB) error {
	var configured, groups, deleted int64
	if err := tx.Model(&Option{}).Where("key = ?", setting.OptionKeyGroupRatio).Count(&configured).Error; err != nil {
		return err
	}
	if err := tx.Model(&ServiceGroupConfig{}).Count(&groups).Error; err != nil {
		return err
	}
	if err := tx.Model(&ServiceGroupDeletion{}).Count(&deleted).Error; err != nil {
		return err
	}
	if configured+groups+deleted > 0 {
		return nil
	}
	ratios := map[string]float64{"default": 1}
	for _, preset := range []struct {
		id, multiplier string
		ratio          float64
		success, p95   int64
	}{
		{"ultra", "1", 1, 995000, 800},
		{"pro", "0.8", 0.8, 990000, 1500},
		{"standard", "0.6", 0.6, 975000, 3000},
		{"lite", "0.4", 0.4, 975000, 3000},
	} {
		group := ServiceGroupConfig{
			GroupID: preset.id, Name: preset.id, Enabled: true, Visibility: "common", Multiplier: preset.multiplier, Revision: 1,
			SLASuccessPPM: &preset.success, SLAP95MS: &preset.p95, SLARevision: 1, SLAUpdatedAt: time.Now().Unix(),
		}
		if err := tx.Create(&group).Error; err != nil {
			return err
		}
		ratios[preset.id] = preset.ratio
	}
	value, err := json.Marshal(ratios)
	if err != nil {
		return err
	}
	return tx.Create(&Option{Key: setting.OptionKeyGroupRatio, Value: string(value)}).Error
}

package model

import (
	"context"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"

	"starhub/setting"
)

type ServiceGroupCatalog struct {
	Groups        []ServiceGroupConfig
	OptionVersion int64
	GroupRatios   map[string]float64
}

// ReadServiceGroupCatalog observes metadata, configured ratios and version in one SQLite snapshot.
func ReadServiceGroupCatalog(ctx context.Context) (*ServiceGroupCatalog, error) {
	result := &ServiceGroupCatalog{Groups: []ServiceGroupConfig{}, GroupRatios: map[string]float64{}}
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var revision OptionRevision
		if err := tx.First(&revision, 1).Error; err != nil {
			return err
		}
		if revision.Version < 0 || revision.Version > maxServiceGroupRevision {
			return fmt.Errorf("option version is unavailable")
		}
		result.OptionVersion = revision.Version
		var option Option
		if err := tx.Where("`key` = ?", setting.OptionKeyGroupRatio).First(&option).Error; err != nil {
			return err
		}
		normalized, err := setting.NormalizeOption(setting.OptionKeyGroupRatio, option.Value)
		if err != nil {
			return err
		}
		if err := json.Unmarshal([]byte(normalized), &result.GroupRatios); err != nil {
			return err
		}
		if result.GroupRatios == nil {
			result.GroupRatios = map[string]float64{}
		}
		return tx.Order("group_id").Find(&result.Groups).Error
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

package model

import (
	"errors"
	"fmt"

	"gorm.io/gorm"

	"starhub/common"
)

// SetChannelResourcesEnabled changes only availability, using the latest stored keys.
// Explicitly enabling a channel restores all keys, including automatically disabled keys.
func SetChannelResourcesEnabled(id int64, enabled bool) (*Channel, error) {
	var ch Channel
	err := mutateChannelConfiguration(func(tx *gorm.DB) error {
		if err := tx.First(&ch, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrChannelNotFound
			}
			return err
		}
		ch.Status = common.ChannelStatusManuallyDisabled
		if enabled {
			ch.Status = common.ChannelStatusEnabled
			ch.MultiKeyStatus = ""
			if err := validateTaskPluginChannel(tx, &ch); err != nil {
				return fmt.Errorf("%w: %v", ErrTaskPluginConfiguration, err)
			}
		}
		if err := tx.Model(&Channel{}).Where("id = ?", id).Updates(map[string]any{
			"status": ch.Status, "multi_key_status": ch.MultiKeyStatus,
		}).Error; err != nil {
			return err
		}
		return rebuildAbilities(tx, &ch)
	})
	if err != nil {
		return nil, err
	}
	return &ch, nil
}

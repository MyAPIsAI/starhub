package model

import (
	"errors"
	"fmt"

	"gorm.io/gorm"

	"starhub/common"
)

// SetChannelResourcesEnabled changes model availability without restoring disabled keys.
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
			if err := validateTaskPluginChannel(tx, &ch); err != nil {
				return fmt.Errorf("%w: %v", ErrTaskPluginConfiguration, err)
			}
			states, err := readChannelResourceStates(tx, []Channel{ch})
			if err != nil {
				return err
			}
			names := make([]string, 0, states[id].Total)
			for _, resource := range states[id].Resources {
				names = append(names, resource.Model)
			}
			if len(names) > 0 {
				if err := tx.Where("channel_id = ? AND model IN ?", id, names).Delete(&ChannelResourceDisable{}).Error; err != nil {
					return err
				}
			}
		}
		if err := tx.Model(&Channel{}).Where("id = ?", id).Updates(map[string]any{
			"status": ch.Status,
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

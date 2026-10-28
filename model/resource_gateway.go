package model

import (
	"errors"
	"fmt"

	"gorm.io/gorm"

	"starhub/common"
)

// Managed rows are durable ownership markers. Revocation never removes the marker,
// so ordinary channel rebuilds and same-ID recreation cannot restore legacy grants.
type ResourceGatewayManagedChannel struct {
	ChannelID       int64  `gorm:"primaryKey;autoIncrement:false"`
	SourceChannelID string `gorm:"size:128;not null"`
	Active          bool   `gorm:"not null;default:false"`
}

type ResourceGatewayRoute struct {
	ChannelID int64  `gorm:"primaryKey;autoIncrement:false"`
	Model     string `gorm:"primaryKey;size:128"`
	Group     string `gorm:"column:group;primaryKey;size:64"`
}

// resourceGatewayAbilityCondition is applied in SQL before candidate limits.
// Aliases come only from static queries in this package, never request values.
func resourceGatewayAbilityCondition(alias string) string {
	return fmt.Sprintf(`(NOT EXISTS (
		SELECT 1 FROM resource_gateway_managed_channels rgm WHERE rgm.channel_id = %[1]s.channel_id
	) OR EXISTS (
		SELECT 1 FROM resource_gateway_managed_channels rgm
		JOIN resource_gateway_routes rgr ON rgr.channel_id = rgm.channel_id
		WHERE rgm.channel_id = %[1]s.channel_id AND rgm.active = 1
		AND rgr.model = %[1]s.model AND rgr."group" = %[1]s."group"
	))`, alias)
}

func addResourceGatewayAbilities(tx *gorm.DB, channel *Channel) (bool, error) {
	var managed ResourceGatewayManagedChannel
	err := tx.First(&managed, "channel_id = ?", channel.ID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !managed.Active {
		return true, nil
	}
	var routes []ResourceGatewayRoute
	if err := tx.Where("channel_id = ?", channel.ID).Find(&routes).Error; err != nil {
		return true, err
	}
	abilities := make([]Ability, 0, len(routes))
	for _, route := range routes {
		abilities = append(abilities, Ability{ChannelID: channel.ID, Model: route.Model, Group: route.Group, Enabled: channel.Status == common.ChannelStatusEnabled, Priority: channel.Priority, Weight: channel.Weight})
	}
	if len(abilities) == 0 {
		return true, nil
	}
	return true, tx.CreateInBatches(abilities, 100).Error
}

func revokeResourceGatewayChannel(tx *gorm.DB, channelID int64) error {
	if err := tx.Model(&ResourceGatewayManagedChannel{}).Where("channel_id = ?", channelID).Update("active", false).Error; err != nil {
		return err
	}
	return tx.Where("channel_id = ?", channelID).Delete(&ResourceGatewayRoute{}).Error
}

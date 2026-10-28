package model

import (
	"errors"
	"fmt"
	"slices"

	"gorm.io/gorm"

	"starhub/common"
)

// Managed rows are durable ownership markers. Revocation never removes the marker,
// so ordinary channel rebuilds and same-ID recreation cannot restore legacy grants.
type ResourceGatewayManagedChannel struct {
	ChannelID       int64  `gorm:"primaryKey;autoIncrement:false"`
	SourceChannelID string `gorm:"size:128;not null"`
	Active          bool   `gorm:"not null;default:false"`
	Profile         string `gorm:"size:64;not null;default:''"`
}

type ResourceGatewayRoute struct {
	ChannelID         int64  `gorm:"primaryKey;autoIncrement:false"`
	Model             string `gorm:"primaryKey;size:128"`
	Group             string `gorm:"column:group;primaryKey;size:64"`
	Capabilities      string `gorm:"type:text;not null;default:'[]'"`
	ContextLength     int64  `gorm:"not null;default:0"`
	GroupDefinitionID int64  `gorm:"not null;default:0"`
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
		AND rgm.profile = 'openai_text_exact_v1'
		AND rgr.capabilities IN ('["text"]', '["code"]', '["code","text"]')
		AND rgr.context_length BETWEEN 0 AND 9007199254740991
		AND EXISTS (SELECT 1 FROM channels rgc WHERE rgc.id = rgm.channel_id AND rgc.type = 1)
		AND `+resourceGatewayGroupCondition("rgr")+`
		AND rgr.model = %[1]s.model AND rgr."group" = %[1]s."group"
	))`, alias)
}

func resourceGatewayGroupCondition(alias string) string {
	return fmt.Sprintf(`(NOT EXISTS (
		SELECT 1 FROM service_group_deletions rgd WHERE rgd.group_id = %[1]s."group"
	) AND (
		(%[1]s.group_definition_id = 0 AND NOT EXISTS (
			SELECT 1 FROM service_group_configs rgs WHERE rgs.group_id = %[1]s."group"
		)) OR EXISTS (
			SELECT 1 FROM service_group_configs rgs
			WHERE rgs.group_id = %[1]s."group" AND rgs.id = %[1]s.group_definition_id
			AND rgs.id > 0 AND rgs.enabled = 1 AND rgs.visibility IN ('common', 'custom')
		)
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
	if !managed.Active || managed.Profile != ResourceGatewayTextProfile || channel.Type != common.DefaultChannelType {
		return true, nil
	}
	var routes []ResourceGatewayRoute
	if err := tx.Where("channel_id = ?", channel.ID).Where(resourceGatewayGroupCondition("resource_gateway_routes")).Find(&routes).Error; err != nil {
		return true, err
	}
	abilities := make([]Ability, 0, len(routes))
	for _, route := range routes {
		if resourceGatewayPolicyForRoute(route) == nil {
			continue
		}
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

func invalidateResourceGatewaySources(tx *gorm.DB, sources map[string]bool) error {
	if len(sources) == 0 {
		return nil
	}
	ids := make([]string, 0, len(sources))
	for id := range sources {
		ids = append(ids, id)
	}
	var managed []ResourceGatewayManagedChannel
	if err := tx.Where("source_channel_id IN ? AND active = ?", ids, true).Find(&managed).Error; err != nil {
		return err
	}
	if len(managed) == 0 {
		return nil
	}
	if err := advanceChannelConfigurationRevision(tx); err != nil {
		return err
	}
	for _, entry := range managed {
		if err := revokeResourceGatewayChannel(tx, entry.ChannelID); err != nil {
			return err
		}
		if err := tx.Where("channel_id = ?", entry.ChannelID).Delete(&Ability{}).Error; err != nil {
			return err
		}
	}
	return nil
}

func sameResourceSet(a, b []string) bool {
	first, second := slices.Clone(a), slices.Clone(b)
	slices.Sort(first)
	slices.Sort(second)
	return slices.Equal(first, second)
}

func sameResourceGatewayIdentity(a, b ResourceInstance) bool {
	return a.ID == b.ID && a.ChannelID == b.ChannelID && a.CanonicalModel == b.CanonicalModel && a.UpstreamModel == b.UpstreamModel && a.Status == b.Status && a.ContextLength == b.ContextLength && sameResourceSet(a.Protocols, b.Protocols) && sameResourceSet(a.Capabilities, b.Capabilities)
}

func invalidatedResourceGatewaySources(previous *ResourceSnapshot, next ResourceSnapshot) map[string]bool {
	changed := map[string]bool{}
	if previous == nil {
		return changed
	}
	channels := map[string]ResourceChannel{}
	resources := map[string]ResourceInstance{}
	for _, channel := range next.Channels {
		channels[channel.ID] = channel
	}
	for _, resource := range next.Instances {
		resources[resource.ID] = resource
	}
	for _, prior := range previous.Channels {
		current, exists := channels[prior.ID]
		if !exists || prior.Enabled != current.Enabled || prior.Provider != current.Provider {
			changed[prior.ID] = true
		}
	}
	for _, prior := range previous.Instances {
		current, exists := resources[prior.ID]
		if !exists || !sameResourceGatewayIdentity(prior, current) {
			changed[prior.ChannelID] = true
		}
	}
	return changed
}

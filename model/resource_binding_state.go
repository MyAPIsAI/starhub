package model

import (
	"context"
	"fmt"
	"strconv"

	"gorm.io/gorm"
)

const ResourceBindingStateSchema = "managed_gateway_bindings_v1"

type ResourceBindingState struct {
	Schema           string                  `json:"schema"`
	ResourceRevision int64                   `json:"resource_revision"`
	ChannelRevision  int64                   `json:"channel_revision"`
	OptionVersion    int64                   `json:"option_version"`
	Page             int                     `json:"page"`
	PageSize         int                     `json:"page_size"`
	Total            int64                   `json:"total"`
	Bindings         []ResourceBindingStatus `json:"bindings"`
	ReadOnly         bool                    `json:"read_only"`
}

type ResourceBindingStatus struct {
	GatewayChannelID       string `json:"gateway_channel_id"`
	GatewayName            string `json:"gateway_name"`
	GatewayPresent         bool   `json:"gateway_present"`
	GatewayStatus          int    `json:"gateway_status"`
	GatewayType            int    `json:"gateway_type"`
	SourceChannelID        string `json:"source_channel_id"`
	SourcePresent          bool   `json:"source_present"`
	SourceEnabled          bool   `json:"source_enabled"`
	ConfiguredActive       bool   `json:"configured_active"`
	Profile                string `json:"profile"`
	PublicationID          string `json:"publication_id"`
	StoredPairCount        int64  `json:"stored_pair_count"`
	RouteEligiblePairCount int64  `json:"route_eligible_pair_count"`
}

// Eligible pairs pass current persisted routing gates, not user authorization,
// balance, runtime health, context estimation or provider acceptance checks.
func ReadResourceBindingState(ctx context.Context, page, pageSize int) (*ResourceBindingState, error) {
	if page < 1 || page > 1000000 || pageSize < 1 || pageSize > 100 {
		return nil, ErrInvalidResource
	}
	var result *ResourceBindingState
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		state, err := readResourcePublicationState(tx)
		if err != nil {
			return err
		}
		value := &ResourceBindingState{Schema: ResourceBindingStateSchema, ResourceRevision: state.context.ResourceRevision, ChannelRevision: state.context.ChannelRevision, OptionVersion: state.context.OptionVersion, Page: page, PageSize: pageSize, Bindings: []ResourceBindingStatus{}, ReadOnly: true}
		if err := tx.Model(&ResourceGatewayManagedChannel{}).Count(&value.Total).Error; err != nil {
			return err
		}
		if value.Total < 0 || value.Total > maxResourceRevision {
			return fmt.Errorf("binding count is unavailable")
		}
		var owners []ResourceGatewayManagedChannel
		if err := tx.Order("channel_id").Limit(pageSize).Offset((page - 1) * pageSize).Find(&owners).Error; err != nil {
			return err
		}
		ids := make([]int64, 0, len(owners))
		for _, owner := range owners {
			if owner.ChannelID <= 0 || owner.PublicationID < 0 || (owner.SourceChannelID != "" && !resourceText(owner.SourceChannelID, 128, true)) {
				return fmt.Errorf("binding state is unavailable")
			}
			ids = append(ids, owner.ChannelID)
		}
		stored, eligible := map[int64]int64{}, map[int64]int64{}
		if len(ids) > 0 {
			var counts []struct {
				ChannelID int64
				Count     int64
			}
			if err := tx.Model(&ResourceGatewayRoute{}).Select("channel_id, COUNT(*) AS count").Where("channel_id IN ?", ids).Group("channel_id").Scan(&counts).Error; err != nil {
				return err
			}
			for _, count := range counts {
				stored[count.ChannelID] = count.Count
			}
			pricedGroups := []string{}
			for group, ratio := range state.ratios {
				if ratio > 0 {
					pricedGroups = append(pricedGroups, group)
				}
			}
			if len(pricedGroups) > 0 {
				counts = nil
				if err := tx.Table("abilities AS a").Select("a.channel_id, COUNT(*) AS count").Joins("JOIN channels AS c ON c.id = a.channel_id").Where("a.channel_id IN ? AND a.enabled = 1 AND c.status = 1", ids).Where("a.\"group\" IN ?", pricedGroups).Where(resourceGatewayAbilityCondition("a")).Group("a.channel_id").Scan(&counts).Error; err != nil {
					return err
				}
				for _, count := range counts {
					eligible[count.ChannelID] = count.Count
				}
			}
		}
		channels := map[string]ResourceGatewayChannel{}
		for _, channel := range state.context.Channels {
			channels[channel.ID] = channel
		}
		sources := map[string]ResourceChannel{}
		if state.catalog.Snapshot != nil {
			for _, source := range state.catalog.Snapshot.Channels {
				sources[source.ID] = source
			}
		}
		for _, owner := range owners {
			id := strconv.FormatInt(owner.ChannelID, 10)
			channel, present := channels[id]
			source, sourcePresent := sources[owner.SourceChannelID]
			if stored[owner.ChannelID] < 0 || stored[owner.ChannelID] > maxResourceRevision || eligible[owner.ChannelID] < 0 || eligible[owner.ChannelID] > stored[owner.ChannelID] {
				return fmt.Errorf("binding route counts are unavailable")
			}
			value.Bindings = append(value.Bindings, ResourceBindingStatus{GatewayChannelID: id, GatewayName: channel.Name, GatewayPresent: present, GatewayStatus: channel.Status, GatewayType: channel.Type, SourceChannelID: owner.SourceChannelID, SourcePresent: sourcePresent, SourceEnabled: sourcePresent && source.Enabled, ConfiguredActive: owner.Active, Profile: owner.Profile, PublicationID: strconv.FormatInt(owner.PublicationID, 10), StoredPairCount: stored[owner.ChannelID], RouteEligiblePairCount: eligible[owner.ChannelID]})
		}
		result = value
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

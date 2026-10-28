package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"

	"gorm.io/gorm"

	"starhub/common"
	"starhub/setting"
)

const ResourcePublicationPreviewSchema = "exact_model_pairs_preview_v1"

const maxResourcePublicationPairs = 10000

type ResourceGatewayChannel struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Type   int    `json:"type"`
	Status int    `json:"status"`
}

type ResourcePublicationContext struct {
	Schema                string                   `json:"schema"`
	ResourceRevision      int64                    `json:"resource_revision"`
	ChannelRevision       int64                    `json:"channel_revision"`
	OptionVersion         int64                    `json:"option_version"`
	Channels              []ResourceGatewayChannel `json:"channels"`
	PreviewOnly           bool                     `json:"preview_only"`
	AffectsGatewayRouting bool                     `json:"affects_gateway_routing"`
}

// Bindings are explicit: inventory source IDs are never interpreted as gateway IDs.
type ResourceChannelBinding struct {
	SourceChannelID  string `json:"source_channel_id"`
	GatewayChannelID string `json:"gateway_channel_id"`
}

type ResourcePublicationPrecondition struct {
	ResourceRevision int64
	ChannelRevision  int64
	OptionVersion    int64
}

type ResourcePublicationPair struct {
	GatewayChannelID string `json:"gateway_channel_id"`
	Model            string `json:"model"`
	Group            string `json:"group"`
}

type ResourcePublicationIssue struct {
	ResourceID      string `json:"resource_id,omitempty"`
	SourceChannelID string `json:"source_channel_id,omitempty"`
	Group           string `json:"group,omitempty"`
	Code            string `json:"code"`
}

type ResourcePublicationExistingPair struct {
	ResourcePublicationPair
	Enabled bool `json:"enabled"`
}

type ResourcePublicationPreview struct {
	ResourcePublicationContext
	Ready         bool                              `json:"ready"`
	Bindings      []ResourceChannelBinding          `json:"bindings"`
	ProposedPairs []ResourcePublicationPair         `json:"proposed_pairs"`
	ExistingPairs []ResourcePublicationExistingPair `json:"existing_pairs"`
	Issues        []ResourcePublicationIssue        `json:"issues"`
}

type resourcePublicationState struct {
	context ResourcePublicationContext
	catalog *ResourceCatalog
	groups  map[string]ServiceGroupConfig
	deleted map[string]bool
	ratios  map[string]float64
}

func readResourcePublicationState(tx *gorm.DB) (*resourcePublicationState, error) {
	catalog, err := readResourceCatalog(tx)
	if err != nil {
		return nil, err
	}
	var channels ChannelConfigurationState
	if err := tx.First(&channels, 1).Error; err != nil {
		return nil, err
	}
	var options OptionRevision
	if err := tx.First(&options, 1).Error; err != nil {
		return nil, err
	}
	for _, version := range []int64{catalog.Revision, channels.Revision, options.Version} {
		if version < 0 || version > maxResourceRevision {
			return nil, fmt.Errorf("publication context version is unavailable")
		}
	}
	result := &resourcePublicationState{
		context: ResourcePublicationContext{Schema: ResourcePublicationPreviewSchema, ResourceRevision: catalog.Revision, ChannelRevision: channels.Revision, OptionVersion: options.Version, Channels: []ResourceGatewayChannel{}, PreviewOnly: true},
		catalog: catalog, groups: map[string]ServiceGroupConfig{}, deleted: map[string]bool{}, ratios: map[string]float64{},
	}
	var rows []Channel
	// Never load credential or endpoint columns into publication context objects.
	if err := tx.Select("id", "name", "type", "status").Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		result.context.Channels = append(result.context.Channels, ResourceGatewayChannel{ID: strconv.FormatInt(row.ID, 10), Name: row.Name, Type: row.Type, Status: row.Status})
	}
	var ratio Option
	if err := tx.Where("`key` = ?", setting.OptionKeyGroupRatio).First(&ratio).Error; err != nil {
		return nil, err
	}
	normalized, err := setting.NormalizeOption(setting.OptionKeyGroupRatio, ratio.Value)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(normalized), &result.ratios); err != nil || result.ratios == nil {
		return nil, fmt.Errorf("group pricing is unavailable")
	}
	var groups []ServiceGroupConfig
	if err := tx.Find(&groups).Error; err != nil {
		return nil, err
	}
	for _, group := range groups {
		result.groups[group.GroupID] = group
	}
	var deleted []ServiceGroupDeletion
	if err := tx.Find(&deleted).Error; err != nil {
		return nil, err
	}
	for _, group := range deleted {
		result.deleted[group.GroupID] = true
	}
	return result, nil
}

func ReadResourcePublicationContext(ctx context.Context) (*ResourcePublicationContext, error) {
	var result *ResourcePublicationContext
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		state, err := readResourcePublicationState(tx)
		if err == nil {
			result = &state.context
		}
		return err
	})
	return result, err
}

// PreviewResourcePublication is read-only and is not an authorization token for a later publish.
// A publisher must revalidate all dependencies and replace exact pairs atomically.
func PreviewResourcePublication(ctx context.Context, expected ResourcePublicationPrecondition, bindings []ResourceChannelBinding) (*ResourcePublicationPreview, error) {
	for _, version := range []int64{expected.ResourceRevision, expected.ChannelRevision, expected.OptionVersion} {
		if version < 0 || version > maxResourceRevision {
			return nil, ErrInvalidResource
		}
	}
	if len(bindings) == 0 || len(bindings) > 500 {
		return nil, ErrInvalidResource
	}
	bound := map[string]int64{}
	targets := map[int64]bool{}
	for _, binding := range bindings {
		id, err := strconv.ParseInt(binding.GatewayChannelID, 10, 64)
		if !resourceText(binding.SourceChannelID, 128, true) || bound[binding.SourceChannelID] != 0 || err != nil || id <= 0 || strconv.FormatInt(id, 10) != binding.GatewayChannelID || targets[id] {
			return nil, ErrInvalidResource
		}
		bound[binding.SourceChannelID], targets[id] = id, true
	}
	var result *ResourcePublicationPreview
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		state, err := readResourcePublicationState(tx)
		if err != nil {
			return err
		}
		if expected.ResourceRevision != state.context.ResourceRevision || expected.ChannelRevision != state.context.ChannelRevision || expected.OptionVersion != state.context.OptionVersion {
			return ErrResourceConflict
		}
		result = &ResourcePublicationPreview{ResourcePublicationContext: state.context, Bindings: append([]ResourceChannelBinding{}, bindings...), ProposedPairs: []ResourcePublicationPair{}, ExistingPairs: []ResourcePublicationExistingPair{}, Issues: []ResourcePublicationIssue{}}
		sort.Slice(result.Bindings, func(i, j int) bool { return result.Bindings[i].SourceChannelID < result.Bindings[j].SourceChannelID })
		gateway := map[string]ResourceGatewayChannel{}
		for _, ch := range state.context.Channels {
			gateway[ch.ID] = ch
		}
		sources := map[string]ResourceChannel{}
		if state.catalog.Snapshot != nil {
			for _, source := range state.catalog.Snapshot.Channels {
				sources[source.ID] = source
			}
		}
		for _, binding := range result.Bindings {
			if _, exists := sources[binding.SourceChannelID]; !exists {
				result.Issues = append(result.Issues, ResourcePublicationIssue{SourceChannelID: binding.SourceChannelID, Code: "unknown_source_channel"})
			}
			if _, exists := gateway[binding.GatewayChannelID]; !exists {
				result.Issues = append(result.Issues, ResourcePublicationIssue{SourceChannelID: binding.SourceChannelID, Code: "unknown_gateway_channel"})
			}
		}
		pairs := map[ResourcePublicationPair]bool{}
		if state.catalog.Snapshot != nil {
			for _, resource := range state.catalog.Snapshot.Instances {
				placement := state.catalog.Placements[resource.ID]
				if !placement.Enabled {
					continue
				}
				issue := func(code, group string) {
					result.Issues = append(result.Issues, ResourcePublicationIssue{ResourceID: resource.ID, SourceChannelID: resource.ChannelID, Group: group, Code: code})
				}
				id, exists := bound[resource.ChannelID]
				if !exists {
					issue("unbound_source_channel", "")
					continue
				}
				ch, exists := gateway[strconv.FormatInt(id, 10)]
				if !exists {
					continue
				}
				if !sources[resource.ChannelID].Enabled || resource.Status != "available" || ch.Status != common.ChannelStatusEnabled {
					issue("unavailable_resource_or_channel", "")
					continue
				}
				if placement.ModelID == nil || *placement.ModelID != resource.CanonicalModel || resource.CanonicalModel != resource.UpstreamModel {
					issue("model_alias_not_supported", "")
					continue
				}
				if !resourceText(*placement.ModelID, 128, true) {
					issue("gateway_model_id_not_supported", "")
					continue
				}
				if ch.Type != common.DefaultChannelType || len(resource.Protocols) != 1 || resource.Protocols[0] != "OpenAI" || !previewTextCapabilities(resource.Capabilities) {
					issue("protocol_or_capability_not_supported", "")
					continue
				}
				for _, group := range placement.GroupIDs {
					definition, defined := state.groups[group]
					if !resourceText(group, 64, true) || state.ratios[group] <= 0 || state.deleted[group] || (defined && (!definition.Enabled || (definition.Visibility != "common" && definition.Visibility != "custom"))) {
						issue("group_unavailable_or_unpriced", group)
						continue
					}
					pairs[ResourcePublicationPair{GatewayChannelID: ch.ID, Model: *placement.ModelID, Group: group}] = true
					if len(pairs) > maxResourcePublicationPairs {
						return fmt.Errorf("%w: publication preview exceeds 10000 pairs", ErrInvalidResource)
					}
				}
			}
		}
		for pair := range pairs {
			result.ProposedPairs = append(result.ProposedPairs, pair)
		}
		if len(result.ProposedPairs) == 0 {
			result.Issues = append(result.Issues, ResourcePublicationIssue{Code: "no_supported_pairs"})
		}
		ids := make([]int64, 0, len(targets))
		for id := range targets {
			ids = append(ids, id)
		}
		var abilities []Ability
		if err := tx.Where("channel_id IN ?", ids).Limit(maxResourcePublicationPairs + 1).Find(&abilities).Error; err != nil {
			return err
		}
		if len(abilities) > maxResourcePublicationPairs {
			return fmt.Errorf("%w: publication preview exceeds 10000 pairs", ErrInvalidResource)
		}
		for _, ability := range abilities {
			result.ExistingPairs = append(result.ExistingPairs, ResourcePublicationExistingPair{ResourcePublicationPair: ResourcePublicationPair{GatewayChannelID: strconv.FormatInt(ability.ChannelID, 10), Model: ability.Model, Group: ability.Group}, Enabled: ability.Enabled})
		}
		sortPublicationPairs(result.ProposedPairs)
		sort.Slice(result.ExistingPairs, func(i, j int) bool {
			return publicationPairLess(result.ExistingPairs[i].ResourcePublicationPair, result.ExistingPairs[j].ResourcePublicationPair)
		})
		sort.Slice(result.Issues, func(i, j int) bool {
			a, b := result.Issues[i], result.Issues[j]
			if a.ResourceID != b.ResourceID {
				return a.ResourceID < b.ResourceID
			}
			if a.SourceChannelID != b.SourceChannelID {
				return a.SourceChannelID < b.SourceChannelID
			}
			if a.Group != b.Group {
				return a.Group < b.Group
			}
			return a.Code < b.Code
		})
		result.Ready = len(result.Issues) == 0
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("publication context is unavailable")
	}
	return result, err
}

func previewTextCapabilities(capabilities []string) bool {
	if len(capabilities) == 0 {
		return false
	}
	for _, capability := range capabilities {
		if capability != "text" && capability != "code" {
			return false
		}
	}
	return true
}

func sortPublicationPairs(pairs []ResourcePublicationPair) {
	sort.Slice(pairs, func(i, j int) bool {
		return publicationPairLess(pairs[i], pairs[j])
	})
}

func publicationPairLess(a, b ResourcePublicationPair) bool {
	if a.GatewayChannelID != b.GatewayChannelID {
		return a.GatewayChannelID < b.GatewayChannelID
	}
	if a.Model != b.Model {
		return a.Model < b.Model
	}
	return a.Group < b.Group
}

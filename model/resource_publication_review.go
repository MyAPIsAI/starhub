package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"gorm.io/gorm"
)

func mergeResourcePublicationRoute(first, second ResourcePublicationRoute) ResourcePublicationRoute {
	result := first
	result.Capabilities = []string{}
	for _, capability := range first.Capabilities {
		for _, candidate := range second.Capabilities {
			if capability == candidate {
				result.Capabilities = append(result.Capabilities, capability)
				break
			}
		}
	}
	if second.ContextLength > 0 && (result.ContextLength == 0 || second.ContextLength < result.ContextLength) {
		result.ContextLength = second.ContextLength
	}
	if result.GroupDefinitionID != second.GroupDefinitionID {
		result.Capabilities = nil
	}
	return result
}

// The digest identifies the complete review snapshot, not a bearer credential.
// A future publisher must recompute this under its writer before changing grants.
func readResourcePublicationOwners(tx *gorm.DB, bindings []ResourceChannelBinding) ([]ResourceGatewayManagedChannel, error) {
	targets := make([]int64, 0, len(bindings))
	sources := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		id, _ := strconv.ParseInt(binding.GatewayChannelID, 10, 64)
		targets = append(targets, id)
		sources = append(sources, binding.SourceChannelID)
	}
	byID := map[int64]ResourceGatewayManagedChannel{}
	for _, query := range []*gorm.DB{tx.Where("channel_id IN ?", targets), tx.Where("source_channel_id IN ?", sources)} {
		var rows []ResourceGatewayManagedChannel
		if err := query.Limit(maxResourcePublicationPairs + 1).Find(&rows).Error; err != nil {
			return nil, err
		}
		if len(rows) > maxResourcePublicationPairs {
			return nil, fmt.Errorf("%w: publication ownership exceeds preview limits", ErrInvalidResource)
		}
		for _, row := range rows {
			byID[row.ChannelID] = row
		}
	}
	if len(byID) > maxResourcePublicationPairs {
		return nil, fmt.Errorf("%w: publication ownership exceeds preview limits", ErrInvalidResource)
	}
	result := make([]ResourceGatewayManagedChannel, 0, len(byID))
	for _, row := range byID {
		result = append(result, row)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ChannelID < result[j].ChannelID })
	return result, nil
}

func sealResourcePublicationPreview(state *resourcePublicationState, preview *ResourcePublicationPreview, owners []ResourceGatewayManagedChannel) error {
	type groupGuard struct {
		Group        string
		DefinitionID string
		Revision     int64
		Enabled      bool
		Visibility   string
	}
	groups := map[string]bool{}
	for _, route := range preview.ProposedRoutes {
		groups[route.Group] = true
	}
	guards := make([]groupGuard, 0, len(groups))
	for group := range groups {
		definition := state.groups[group]
		guards = append(guards, groupGuard{Group: group, DefinitionID: strconv.FormatInt(definition.ID, 10), Revision: definition.Revision, Enabled: definition.Enabled, Visibility: definition.Visibility})
	}
	sort.Slice(guards, func(i, j int) bool { return guards[i].Group < guards[j].Group })
	encoded, err := json.Marshal(struct {
		Context  ResourcePublicationContext
		Bindings []ResourceChannelBinding
		Routes   []ResourcePublicationRoute
		Existing []ResourcePublicationExistingPair
		Groups   []groupGuard
		Owners   []ResourceGatewayManagedChannel
		Issues   []ResourcePublicationIssue
	}{preview.ResourcePublicationContext, preview.Bindings, preview.ProposedRoutes, preview.ExistingPairs, guards, owners, preview.Issues})
	if err != nil {
		return err
	}
	digest := sha256.Sum256(encoded)
	preview.ReviewDigest = hex.EncodeToString(digest[:])
	return nil
}

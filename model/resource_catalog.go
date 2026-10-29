package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"

	"gorm.io/gorm"
)

var (
	ErrResourceConflict = errors.New("resource catalog revision conflict")
	ErrInvalidResource  = errors.New("invalid resource catalog")
)

const maxResourceRevision int64 = 1<<53 - 1

// ResourceCatalogRecord stores imported inventory, separate from gateway credentials.
// Inventory never grants API access automatically. Changes to published resources
// revoke their managed channel until an explicit publication is reviewed again.
type ResourceCatalogRecord struct {
	ID        int    `gorm:"primaryKey"`
	Revision  int64  `gorm:"not null;default:0"`
	Snapshot  string `gorm:"type:text;not null;default:''"`
	UpdatedAt int64  `gorm:"not null;default:0"`
}

type ResourcePlacementRecord struct {
	ResourceID string `gorm:"primaryKey;size:128"`
	ModelID    *string
	GroupIDs   string `gorm:"type:text;not null"`
	Enabled    bool   `gorm:"not null;default:false"`
}

type ResourceChannel struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Enabled  bool   `json:"enabled"`
}

type ResourceInstance struct {
	ID             string   `json:"id"`
	ChannelID      string   `json:"channelId"`
	UpstreamModel  string   `json:"upstreamModel"`
	CanonicalModel string   `json:"canonicalModel"`
	Protocols      []string `json:"protocols"`
	Capabilities   []string `json:"capabilities"`
	ContextLength  int64    `json:"contextLength"`
	Region         string   `json:"region"`
	Status         string   `json:"status"`
	LatencyMS      float64  `json:"latencyMs"`
}

type ResourceSnapshot struct {
	ID          string             `json:"id"`
	Sequence    int64              `json:"sequence"`
	PublishedAt string             `json:"publishedAt"`
	ReceivedAt  string             `json:"receivedAt"`
	Source      string             `json:"source"`
	Channels    []ResourceChannel  `json:"channels"`
	Instances   []ResourceInstance `json:"instances"`
}

type ResourcePlacement struct {
	ModelID  *string  `json:"modelId"`
	GroupIDs []string `json:"groupIds"`
	Enabled  bool     `json:"enabled"`
}

type ResourceCatalog struct {
	Revision              int64                        `json:"revision"`
	Snapshot              *ResourceSnapshot            `json:"snapshot"`
	Placements            map[string]ResourcePlacement `json:"placements"`
	Regions               map[string]ResourceRegion    `json:"regions"`
	UpdatedAt             int64                        `json:"updated_at"`
	AffectsGatewayRouting bool                         `json:"affects_gateway_routing"`
}

func resourceText(value string, max int, required bool) bool {
	return (!required || value != "") && len(value) <= max && value == strings.TrimSpace(value) && !strings.ContainsFunc(value, unicode.IsControl)
}

func concreteResourceModel(value string) bool {
	return resourceText(value, 256, true) && value != "myapis-auto" && value != "veyra-auto"
}

func resourceEnums(values []string, allowed ...string) bool {
	if values == nil || len(values) > len(allowed) {
		return false
	}
	seen := map[string]bool{}
	for _, value := range values {
		found := false
		for _, candidate := range allowed {
			if value == candidate {
				found = true
				break
			}
		}
		if !found || seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}

func validateResourceSnapshot(snapshot ResourceSnapshot) error {
	invalid := func(field string) error { return fmt.Errorf("%w: %s", ErrInvalidResource, field) }
	if !resourceText(snapshot.ID, 128, true) || snapshot.Sequence < 1 || snapshot.Sequence > maxResourceRevision || !resourceText(snapshot.Source, 256, false) {
		return invalid("snapshot identity")
	}
	if _, err := time.Parse(time.RFC3339Nano, snapshot.PublishedAt); err != nil {
		return invalid("publication time")
	}
	if snapshot.Channels == nil || snapshot.Instances == nil || len(snapshot.Channels) > 500 || len(snapshot.Instances) > 10000 {
		return invalid("inventory size")
	}
	channels := map[string]bool{}
	for _, channel := range snapshot.Channels {
		if !resourceText(channel.ID, 128, true) || channels[channel.ID] || !resourceText(channel.Name, 256, true) || !resourceText(channel.Provider, 256, false) {
			return invalid("channel")
		}
		channels[channel.ID] = true
	}
	ids := map[string]bool{}
	for _, instance := range snapshot.Instances {
		if !resourceText(instance.ID, 128, true) || ids[instance.ID] || !channels[instance.ChannelID] || !concreteResourceModel(instance.UpstreamModel) || !concreteResourceModel(instance.CanonicalModel) {
			return invalid("instance identity or reference")
		}
		if !resourceEnums(instance.Protocols, "OpenAI", "Anthropic", "Gemini") || !resourceEnums(instance.Capabilities, resourceAdvancedCapabilities...) {
			return invalid("instance protocol or capability")
		}
		if instance.ContextLength < 0 || instance.ContextLength > maxResourceRevision || math.IsNaN(instance.LatencyMS) || math.IsInf(instance.LatencyMS, 0) || instance.LatencyMS < 0 || instance.LatencyMS > float64(maxResourceRevision) || !resourceText(instance.Region, 128, false) {
			return invalid("instance metadata")
		}
		if instance.Status != "available" && instance.Status != "unavailable" && instance.Status != "retired" {
			return invalid("instance status")
		}
		ids[instance.ID] = true
	}
	return nil
}

func readResourceCatalog(tx *gorm.DB) (*ResourceCatalog, error) {
	var record ResourceCatalogRecord
	if err := tx.First(&record, 1).Error; err != nil {
		return nil, err
	}
	result := &ResourceCatalog{Revision: record.Revision, UpdatedAt: record.UpdatedAt, Placements: map[string]ResourcePlacement{}, Regions: map[string]ResourceRegion{}}
	if record.Snapshot != "" {
		if err := json.Unmarshal([]byte(record.Snapshot), &result.Snapshot); err != nil {
			return nil, err
		}
	}
	var placements []ResourcePlacementRecord
	if err := tx.Order("resource_id").Find(&placements).Error; err != nil {
		return nil, err
	}
	for _, row := range placements {
		groups := []string{}
		if err := json.Unmarshal([]byte(row.GroupIDs), &groups); err != nil {
			return nil, err
		}
		result.Placements[row.ResourceID] = ResourcePlacement{ModelID: row.ModelID, GroupIDs: groups, Enabled: row.Enabled}
	}
	var regions []ResourceRegion
	if err := tx.Order("id").Find(&regions).Error; err != nil {
		return nil, err
	}
	for _, region := range regions {
		result.Regions[region.ID] = region
	}
	var managed int64
	if err := tx.Model(&ResourceGatewayManagedChannel{}).Count(&managed).Error; err != nil {
		return nil, err
	}
	result.AffectsGatewayRouting = managed > 0
	return result, nil
}

func ReadResourceCatalog(ctx context.Context) (*ResourceCatalog, error) {
	var result *ResourceCatalog
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error { var err error; result, err = readResourceCatalog(tx); return err })
	if err != nil {
		return nil, err
	}
	return result, nil
}

// mutateResourceCatalog claims the revision before reading, avoiding SQLite read-to-write upgrades.
// Failed validation or writes roll back the claim together with the inventory and placements.
func mutateResourceCatalog(ctx context.Context, expected int64, mutate func(*gorm.DB, *ResourceCatalog) error) (*ResourceCatalog, error) {
	if expected < 0 || expected >= maxResourceRevision {
		return nil, ErrInvalidResource
	}
	var result *ResourceCatalog
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		claim := tx.Model(&ResourceCatalogRecord{}).Where("id = 1 AND revision = ?", expected).Updates(map[string]any{"revision": expected + 1, "updated_at": time.Now().Unix()})
		if claim.Error != nil {
			return claim.Error
		}
		if claim.RowsAffected != 1 {
			return ErrResourceConflict
		}
		previous, err := readResourceCatalog(tx)
		if err != nil {
			return err
		}
		if err := mutate(tx, previous); err != nil {
			return err
		}
		result, err = readResourceCatalog(tx)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func SaveResourceSnapshot(ctx context.Context, expected int64, snapshot ResourceSnapshot) (*ResourceCatalog, error) {
	if err := validateResourceSnapshot(snapshot); err != nil {
		return nil, err
	}
	snapshot.ReceivedAt = time.Now().UTC().Format(time.RFC3339Nano)
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return nil, err
	}
	if len(encoded) > 1<<20 {
		return nil, ErrInvalidResource
	}
	return mutateResourceCatalog(ctx, expected, func(tx *gorm.DB, previous *ResourceCatalog) error {
		if previous.Snapshot != nil && snapshot.Sequence <= previous.Snapshot.Sequence {
			return ErrResourceConflict
		}
		if err := invalidateResourceGatewaySources(tx, invalidatedResourceGatewaySources(previous.Snapshot, snapshot)); err != nil {
			return err
		}
		old := map[string]ResourceInstance{}
		if previous.Snapshot != nil {
			for _, r := range previous.Snapshot.Instances {
				old[r.ID] = r
			}
		}
		ids := make([]string, 0, len(snapshot.Instances))
		for _, r := range snapshot.Instances {
			ids = append(ids, r.ID)
			row := ResourcePlacementRecord{ResourceID: r.ID, GroupIDs: "[]"}
			if err := tx.Where("resource_id = ?", r.ID).FirstOrCreate(&row).Error; err != nil {
				return err
			}
			prior, exists := old[r.ID]
			if !exists || prior.UpstreamModel != r.UpstreamModel || prior.CanonicalModel != r.CanonicalModel || prior.ChannelID != r.ChannelID || r.Status == "retired" {
				if err := tx.Model(&row).Update("enabled", false).Error; err != nil {
					return err
				}
			}
		}
		stale := tx.Session(&gorm.Session{AllowGlobalUpdate: true})
		if len(ids) > 0 {
			stale = stale.Where("resource_id NOT IN ?", ids)
		}
		if err := stale.Delete(&ResourcePlacementRecord{}).Error; err != nil {
			return err
		}
		return tx.Model(&ResourceCatalogRecord{}).Where("id = 1").Update("snapshot", string(encoded)).Error
	})
}

func SaveResourcePlacement(ctx context.Context, expected int64, id string, placement ResourcePlacement) (*ResourceCatalog, error) {
	if !resourceText(id, 128, true) || placement.GroupIDs == nil || len(placement.GroupIDs) > 100 || (placement.ModelID != nil && !concreteResourceModel(*placement.ModelID)) {
		return nil, ErrInvalidResource
	}
	seen := map[string]bool{}
	for _, group := range placement.GroupIDs {
		if !resourceText(group, 128, true) || seen[group] {
			return nil, ErrInvalidResource
		}
		seen[group] = true
	}
	if placement.Enabled && (placement.ModelID == nil || len(placement.GroupIDs) == 0) {
		return nil, ErrInvalidResource
	}
	return mutateResourceCatalog(ctx, expected, func(tx *gorm.DB, catalog *ResourceCatalog) error {
		if catalog.Snapshot == nil {
			return ErrInvalidResource
		}
		var resource *ResourceInstance
		for _, r := range catalog.Snapshot.Instances {
			if r.ID == id {
				resource = &r
				break
			}
		}
		if resource == nil {
			return ErrInvalidResource
		}
		if placement.Enabled {
			available := false
			for _, c := range catalog.Snapshot.Channels {
				if c.ID == resource.ChannelID {
					available = c.Enabled
				}
			}
			if resource.Status != "available" || !available {
				return ErrInvalidResource
			}
		}
		groups, _ := json.Marshal(placement.GroupIDs)
		prior := catalog.Placements[id]
		sameModel := (prior.ModelID == nil && placement.ModelID == nil) || (prior.ModelID != nil && placement.ModelID != nil && *prior.ModelID == *placement.ModelID)
		if prior.Enabled != placement.Enabled || !sameModel || !sameResourceSet(prior.GroupIDs, placement.GroupIDs) {
			if err := invalidateResourceGatewaySources(tx, map[string]bool{resource.ChannelID: true}); err != nil {
				return err
			}
		}
		return tx.Model(&ResourcePlacementRecord{}).Where("resource_id = ?", id).Updates(map[string]any{"model_id": placement.ModelID, "group_ids": string(groups), "enabled": placement.Enabled}).Error
	})
}

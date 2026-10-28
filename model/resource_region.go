package model

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

var ErrResourceRegionInUse = errors.New("region is referenced by the current resource inventory")

// ResourceRegion describes an inventory region, not a deployed gateway or a routing switch.
// Imported labels without a matching record remain unregistered instead of acquiring defaults.
type ResourceRegion struct {
	ID          string `gorm:"primaryKey;size:128" json:"id"`
	Name        string `gorm:"size:128;not null" json:"name"`
	Description string `gorm:"size:512;not null;default:''" json:"description"`
	UpdatedAt   int64  `gorm:"autoUpdateTime" json:"updated_at"`
}

func SaveResourceRegion(ctx context.Context, expected int64, region ResourceRegion) (*ResourceCatalog, error) {
	if !resourceText(region.ID, 128, true) || !resourceText(region.Name, 128, true) || !resourceText(region.Description, 512, false) {
		return nil, ErrInvalidResource
	}
	return mutateResourceCatalog(ctx, expected, func(tx *gorm.DB, catalog *ResourceCatalog) error {
		if _, exists := catalog.Regions[region.ID]; !exists && len(catalog.Regions) >= 1000 {
			return ErrInvalidResource
		}
		// Update timestamps are server-owned; callers cannot backdate a record.
		region.UpdatedAt = 0
		return tx.Save(&region).Error
	})
}

func DeleteResourceRegion(ctx context.Context, expected int64, id string) (*ResourceCatalog, error) {
	if !resourceText(id, 128, true) {
		return nil, ErrInvalidResource
	}
	return mutateResourceCatalog(ctx, expected, func(tx *gorm.DB, catalog *ResourceCatalog) error {
		if _, exists := catalog.Regions[id]; !exists {
			return ErrInvalidResource
		}
		if catalog.Snapshot != nil {
			for _, resource := range catalog.Snapshot.Instances {
				if resource.Region == id {
					return ErrResourceRegionInUse
				}
			}
		}
		return tx.Delete(&ResourceRegion{}, "id = ?", id).Error
	})
}

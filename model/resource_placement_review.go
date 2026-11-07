package model

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

type ResourceIdentity struct {
	ChannelID      string `json:"channel_id"`
	UpstreamModel  string `json:"upstream_model"`
	CanonicalModel string `json:"canonical_model"`
	Provider       string `json:"provider"`
}

type ResourcePlacementReview struct {
	Before ResourceIdentity `json:"before"`
	After  ResourceIdentity `json:"after"`
}

// Deleted source entries are retained separately and never participate in routing.
type ResourcePlacementArchive struct {
	ID         int64   `gorm:"primaryKey;autoIncrement" json:"id"`
	ResourceID string  `gorm:"index" json:"resource_id"`
	ModelID    *string `json:"model_id"`
	GroupIDs   string  `gorm:"type:text" json:"-"`
	Identity   string  `gorm:"type:text" json:"-"`
	RemovedAt  int64   `json:"removed_at"`
	Sequence   int64   `json:"sequence"`
}

type RemovedResourcePlacement struct {
	ResourcePlacementArchive
	Groups         []string         `json:"group_ids"`
	SourceIdentity ResourceIdentity `json:"identity"`
}

func resourceIdentity(snapshot *ResourceSnapshot, instance ResourceInstance) ResourceIdentity {
	identity := ResourceIdentity{ChannelID: instance.ChannelID, UpstreamModel: instance.UpstreamModel, CanonicalModel: instance.CanonicalModel}
	for _, channel := range snapshot.Channels {
		if channel.ID == instance.ChannelID {
			identity.Provider = channel.Provider
			break
		}
	}
	return identity
}

func archiveRemovedResourcePlacements(tx *gorm.DB, previous *ResourceCatalog, next ResourceSnapshot) error {
	if previous.Snapshot == nil {
		return nil
	}
	present := map[string]bool{}
	for _, instance := range next.Instances {
		present[instance.ID] = true
	}
	for _, instance := range previous.Snapshot.Instances {
		if present[instance.ID] {
			continue
		}
		placement := previous.Placements[instance.ID]
		groups, _ := json.Marshal(placement.GroupIDs)
		identity, _ := json.Marshal(resourceIdentity(previous.Snapshot, instance))
		archive := ResourcePlacementArchive{ResourceID: instance.ID, ModelID: placement.ModelID, GroupIDs: string(groups), Identity: string(identity), RemovedAt: time.Now().Unix(), Sequence: next.Sequence}
		if err := tx.Create(&archive).Error; err != nil {
			return err
		}
	}
	return nil
}

func ClearRemovedResourcePlacement(ctx context.Context, expected, archiveID int64) (*ResourceCatalog, error) {
	if archiveID <= 0 {
		return nil, ErrInvalidResource
	}
	return mutateResourceCatalog(ctx, expected, func(tx *gorm.DB, _ *ResourceCatalog) error {
		result := tx.Delete(&ResourcePlacementArchive{}, archiveID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrResourceConflict
		}
		return nil
	})
}

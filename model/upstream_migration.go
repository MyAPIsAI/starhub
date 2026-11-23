package model

import (
	"errors"

	"gorm.io/gorm"
	"starhub/internal/upstream"
)

// Upgrade the prior protection identity without dropping active cooldowns.
// This runs before traffic or probe workers start. Mixed-version writers must
// not share this state during the identity migration.
func migrateUpstreamIdentities() error {
	var states []upstream.State
	return DB.Order("id").FindInBatches(&states, 200, func(_ *gorm.DB, _ int) error {
		for _, old := range states {
			ch, err := GetChannelByID(old.ChannelID)
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			if old.Generation != ChannelEndpointGeneration(ch) {
				continue
			}
			next := old
			next.Generation = UpstreamEndpointGeneration(ch)
			if next.Capability == "" {
				next.Capability = "chat"
			}
			if next.Scope == "model" {
				next.Scope = "interface"
			}
			next.ID = next.Identity.StateID(next.Scope)
			next.ProbeID, next.LeaseUntil = "", 0
			next.Version++
			if err := DB.Transaction(func(tx *gorm.DB) error {
				variants := []upstream.State{next}
				// The old schema did not record streaming. Keep both variants
				// protected until each protocol has independently recovered.
				if old.Scope == "model" {
					streaming := next
					streaming.Stream = true
					streaming.ID = streaming.Identity.StateID(streaming.Scope)
					variants = append(variants, streaming)
				}
				for _, next := range variants {
					var existing upstream.State
					err := tx.First(&existing, "id = ?", next.ID).Error
					if err == nil {
						next.OpenUntil = max(next.OpenUntil, existing.OpenUntil)
						next.Failures = max(next.Failures, existing.Failures)
						next.Backoff = max(next.Backoff, existing.Backoff)
						next.Version = max(next.Version, existing.Version+1)
					} else if !errors.Is(err, gorm.ErrRecordNotFound) {
						return err
					}
					if err := tx.Save(&next).Error; err != nil {
						return err
					}
				}
				if err := tx.Model(&upstream.Event{}).Where("resource_id = ?", old.ID).Update("resource_id", next.ID).Error; err != nil {
					return err
				}
				return tx.Delete(&upstream.State{}, "id = ?", old.ID).Error
			}); err != nil {
				return err
			}
		}
		return nil
	}).Error
}

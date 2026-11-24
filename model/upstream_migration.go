package model

import (
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
	"starhub/internal/upstream"
)

// migrateRateLimitedUntil backfills the independent rate-limit deadline added
// after Reason was found insufficient. Rows that still expose rate_limited can
// reuse OpenUntil. Rows whose Reason was overwritten are reconstructed from
// recent rate_limited events within the current default cooldown window; older
// history is intentionally not guessed, so those rows lose the extra deadline
// until the next 429.
func migrateRateLimitedUntil() error {
	now := time.Now().Unix()
	if err := DB.Model(&upstream.State{}).
		Where("rate_limited_until = 0 AND reason = ? AND open_until > ?", "rate_limited", now).
		Update("rate_limited_until", gorm.Expr("open_until")).Error; err != nil {
		return err
	}
	rateSeconds := upstream.CurrentPolicy().RateSeconds
	var stored Option
	if err := DB.Where("key = ?", upstream.OptionKey).Limit(1).Find(&stored).Error; err == nil && stored.Value != "" {
		var policy struct {
			RateSeconds int `json:"rate_limit_seconds"`
		}
		if json.Unmarshal([]byte(stored.Value), &policy) == nil && policy.RateSeconds > 0 {
			rateSeconds = policy.RateSeconds
		}
	}
	window := int64(max(1, rateSeconds))
	var events []upstream.Event
	if err := DB.Where("class = ? AND created > ?", "rate_limited", now-window).Order("created").Find(&events).Error; err != nil {
		return err
	}
	deadlines := map[string]int64{}
	for _, event := range events {
		if event.ResourceID == "" {
			continue
		}
		if due := event.Created + window; due > now && due > deadlines[event.ResourceID] {
			deadlines[event.ResourceID] = due
		}
	}
	for id, due := range deadlines {
		if err := DB.Model(&upstream.State{}).Where("id = ? AND rate_limited_until < ?", id, due).
			Update("rate_limited_until", due).Error; err != nil {
			return err
		}
	}
	return nil
}

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

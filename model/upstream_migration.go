package model

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"starhub/internal/upstream"
)

// upstreamMigration records one-time data repairs that must not replay on every
// start. The v2 identifier intentionally differs from the earlier unversioned
// rebuild so databases that already ran it are repaired again.
type upstreamMigration struct {
	ID      string `gorm:"primaryKey;size:64"`
	Applied int64  `gorm:"not null;default:0"`
}

func (upstreamMigration) TableName() string { return "upstream_migrations" }

const upstreamRateLimitMigration = "upstream_rate_limit_deadline_v2"

// normalizeUpstreamNulls runs before AutoMigrate adds NOT NULL defaults.
// Columns added to existing SQLite tables can hold NULL even when the Go field
// reads as zero, so the repair must first materialize those values.
func normalizeUpstreamNulls(db *gorm.DB) error {
	if db.Migrator().HasTable(&upstream.State{}) && db.Migrator().HasColumn(&upstream.State{}, "rate_limited_until") {
		if err := db.Exec("UPDATE upstream_states SET rate_limited_until = 0 WHERE rate_limited_until IS NULL").Error; err != nil {
			return err
		}
	}
	if db.Migrator().HasTable(&upstream.FallbackSlot{}) && db.Migrator().HasColumn(&upstream.FallbackSlot{}, "last_attempt") {
		if err := db.Exec("UPDATE upstream_fallback_slots SET last_attempt = 0 WHERE last_attempt IS NULL").Error; err != nil {
			return err
		}
	}
	return nil
}

// migrateRateLimitedUntil restores independent rate-limit deadlines from durable
// evidence. It runs once under the upstream writer: data changes and the
// completion marker commit together, and later resets or recoveries invalidate
// older rate evidence instead of being overwritten by it.
func migrateRateLimitedUntil() error {
	now := time.Now().Unix()
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&upstream.Guard{}).Where("id = 1").UpdateColumn("id", gorm.Expr("id")).Error; err != nil {
			return err
		}
		var done int64
		if err := tx.Model(&upstreamMigration{}).Where("id = ?", upstreamRateLimitMigration).Count(&done).Error; err != nil {
			return err
		}
		if done > 0 {
			return nil
		}
		if err := rebuildRateLimitDeadlines(tx, now); err != nil {
			return err
		}
		return tx.Create(&upstreamMigration{ID: upstreamRateLimitMigration, Applied: now}).Error
	})
}

func rebuildRateLimitDeadlines(tx *gorm.DB, now int64) error {
	var states []upstream.State
	if err := tx.Where("(rate_limited_until IS NULL OR rate_limited_until = 0) AND (open_until > ? OR reason = ?)", now, "rate_limited").
		Order("id").Find(&states).Error; err != nil {
		return err
	}
	for _, state := range states {
		deadline := int64(0)
		evidenceAt := state.Updated
		if state.Reason == "rate_limited" && state.OpenUntil > now {
			deadline = state.OpenUntil
		}
		// Attempt events store the rate class; probe events keep it in Decision.
		// Both persist the protection deadline in OpenUntil, which is a
		// conservative upper bound when the reason was later overwritten.
		var event upstream.Event
		if err := tx.Where("resource_id = ? AND open_until > ? AND (class = ? OR (kind = ? AND decision = ?))",
			state.ID, now, "rate_limited", "probe", "rate_limited").
			Order("open_until DESC").Limit(1).Find(&event).Error; err != nil {
			return err
		}
		if event.ID != "" {
			if event.OpenUntil > deadline {
				deadline = event.OpenUntil
			}
			evidenceAt = event.Created
		}
		if deadline <= now {
			continue
		}
		// A reset, successful probe, or successful fallback recovery after the
		// rate evidence already cleared it. fallback_recovery_skipped does not
		// count as recovery.
		var later int64
		if err := tx.Model(&upstream.Event{}).Where(
			"resource_id = ? AND created >= ? AND ((kind = ? AND class = ?) OR (kind = ? AND class = ?) OR (kind = ? AND class = ?))",
			state.ID, evidenceAt,
			"reset", "manual_reset",
			"probe", "success",
			"recovery", "fallback_recovered",
		).Count(&later).Error; err != nil {
			return err
		}
		if later > 0 {
			continue
		}
		// Bump the version and release any stale probe lease so an older probe
		// result cannot overwrite the restored protection.
		if err := tx.Model(&upstream.State{}).
			Where("id = ? AND (rate_limited_until IS NULL OR rate_limited_until = 0)", state.ID).
			Updates(map[string]any{
				"rate_limited_until": deadline,
				"version":            gorm.Expr("version + 1"),
				"probe_id":           "",
				"lease_until":        0,
			}).Error; err != nil {
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
						next.RateLimitedUntil = max(next.RateLimitedUntil, existing.RateLimitedUntil)
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

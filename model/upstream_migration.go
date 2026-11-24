package model

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"starhub/internal/upstream"
)

// upstreamMigration records one-time data repairs that must not replay on every
// start. Each repair uses a new identifier so earlier completed migrations do
// not prevent correcting their historical results.
type upstreamMigration struct {
	ID      string `gorm:"primaryKey;size:64"`
	Applied int64  `gorm:"not null;default:0"`
}

func (upstreamMigration) TableName() string { return "upstream_migrations" }

const upstreamRateLimitMigration = "upstream_rate_limit_deadline_v3"

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
	if err := tx.Where("open_until <> 0 OR rate_limited_until <> 0 OR reason = ?", "rate_limited").
		Order("id").Find(&states).Error; err != nil {
		return err
	}
	for _, state := range states {
		// Find the recovery boundary before choosing any rate evidence. An old
		// long deadline must not hide a shorter rate limit after a reset.
		var recovery upstream.Event
		if err := tx.Where(
			"resource_id = ? AND ((kind = ? AND class = ?) OR (kind = ? AND class = ?) OR (kind = ? AND class = ?))",
			state.ID,
			"reset", "manual_reset",
			"probe", "success",
			"recovery", "fallback_recovered",
		).Order("created DESC").Limit(1).Find(&recovery).Error; err != nil {
			return err
		}
		deadline := state.RateLimitedUntil
		if recovery.ID != "" && recovery.Created > state.Updated {
			deadline = 0
		}
		if state.OpenUntil == 0 {
			// Never reopen a closed state from legacy events. Explicit clear
			// reasons also repair cooldowns resurrected by the unversioned rebuild.
			if state.Reason == "manual_reset" || state.Reason == "recovered" || state.Reason == "fallback_recovered" ||
				recovery.ID != "" && recovery.Created >= state.Updated {
				deadline = 0
			}
		} else {
			// Legacy timestamps have only second precision. For an open state,
			// keep same-second evidence conservatively; a closed state above is
			// authoritative when the reset actually happened last.
			query := tx.Where("resource_id = ? AND open_until > ? AND (class = ? OR (kind = ? AND decision = ?))",
				state.ID, now, "rate_limited", "probe", "rate_limited")
			if recovery.ID != "" {
				query = query.Where("created >= ?", recovery.Created)
			}
			var event upstream.Event
			if err := query.Order("open_until DESC").Limit(1).Find(&event).Error; err != nil {
				return err
			}
			if event.ID != "" {
				if recovery.ID != "" {
					// Rebuild after a clear instead of carrying a possibly longer
					// nonzero deadline resurrected from before that boundary.
					deadline = event.OpenUntil
				} else {
					deadline = max(deadline, event.OpenUntil)
				}
			}
			if state.Reason == "rate_limited" && state.OpenUntil > now &&
				(recovery.ID == "" || state.Updated >= recovery.Created) {
				if recovery.ID != "" && event.ID == "" {
					deadline = state.OpenUntil
				} else {
					deadline = max(deadline, state.OpenUntil)
				}
			}
		}
		if deadline == state.RateLimitedUntil {
			continue
		}
		// Bump the version and release any stale probe lease so an older probe
		// result cannot overwrite the restored protection.
		if state.ProbeID != "" {
			if err := tx.Delete(&upstream.ProbeSlot{}, "id = ?", state.ProbeID).Error; err != nil {
				return err
			}
		}
		result := tx.Model(&upstream.State{}).
			Where("id = ? AND version = ?", state.ID, state.Version).
			Updates(map[string]any{
				"rate_limited_until": deadline,
				"version":            gorm.Expr("version + 1"),
				"probe_id":           "",
				"lease_until":        0,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("upstream protection changed during rate-limit migration")
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

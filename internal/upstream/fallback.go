package upstream

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// FallbackSlot limits protected production attempts across replicas sharing a database.
// LastAttempt lets a raised interval immediately extend an existing deadline;
// legacy rows that predate the column keep their original NextAttempt.
type FallbackSlot struct {
	ID          string `gorm:"primaryKey;size:64"`
	NextAttempt int64  `gorm:"not null;default:0"`
	LastAttempt int64  `gorm:"not null;default:0"`
}

func (FallbackSlot) TableName() string { return "upstream_fallback_slots" }

func claimFallback(tx *gorm.DB, identity Identity, p Policy) error {
	id := identity.key("credential")
	var slot FallbackSlot
	err := tx.First(&slot, "id = ?", id).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	now := time.Now().Unix()
	interval := int64(max(1, p.FallbackInterval))
	deadline := slot.NextAttempt
	if slot.LastAttempt > 0 {
		// Recompute from the last attempt with the current interval so an
		// increase constrains the next send; never shorten an existing grant.
		if due := slot.LastAttempt + interval; due > deadline {
			deadline = due
		}
	}
	if deadline > now {
		return ErrBlocked
	}
	return tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&FallbackSlot{ID: id, NextAttempt: now + interval, LastAttempt: now}).Error
}

// Only completed business responses enter this transaction, never handshakes.
func recoverFallback(tx *gorm.DB, a *Attempt, requestID string, status int, committed *Committed) error {
	a.Recovered = map[string]uint64{}
	now := time.Now().Unix()
	// A successful fallback also breaks a run of failures below the open threshold.
	for _, scope := range []string{"model", "interface"} {
		id := a.Identity.key(scope)
		if a.Failures[id] > 0 {
			update := tx.Model(&State{}).Where("id = ? AND version = ? AND open_until = 0 AND (rate_limited_until = 0 OR rate_limited_until <= ?)", id, a.Versions[id], now).Updates(map[string]any{"failures": 0, "reason": "recovered", "updated": now})
			if update.Error != nil {
				return update.Error
			}
			if update.RowsAffected > 0 && committed != nil {
				committed.Failures[id] = 0
			}
		}
	}
	for id, version := range a.Protected {
		eventID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(a.ID+":"+id+":recovery")).String()
		var count int64
		if err := tx.Model(&Event{}).Where("id = ?", eventID).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			continue
		}
		event := Event{ID: eventID, RequestID: requestID, ChannelID: a.ChannelID, Model: a.Model, ResourceID: id, Kind: "recovery", Class: "fallback_recovery_skipped", Decision: "protection_changed", Status: status, Created: now, Started: a.Started, RoutingTrace: a.RoutingTrace}
		var state State
		err := tx.First(&state, "id = ?", id).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		// An in-flight success must not clear a still-valid provider cooldown;
		// the state keeps its protection until the deadline expires.
		if err == nil && state.Version == version && state.OpenUntil != 0 && state.RateLimitedUntil <= now {
			if state.ProbeID != "" {
				if err := tx.Delete(&ProbeSlot{}, "id = ?", state.ProbeID).Error; err != nil {
					return err
				}
			}
			state.Version++
			state.OpenUntil, state.Failures, state.Backoff, state.LeaseUntil = 0, 0, 0, 0
			state.RateLimitedUntil = 0
			state.ProbeID, state.ProbeResult, state.Reason = "", "fallback_recovered", "fallback_recovered"
			state.Updated = event.Created
			if err := tx.Save(&state).Error; err != nil {
				return err
			}
			event.Class, event.Decision = "fallback_recovered", "protection_cleared"
			a.Recovered[id] = state.Version
			if committed != nil {
				committed.Versions[id] = state.Version
				committed.Failures[id] = 0
			}
		}
		event.OpenUntil = state.OpenUntil
		if err := tx.Create(&event).Error; err != nil {
			return err
		}
	}
	return nil
}

// FallbackRecoveryCurrent runs inside the caller's transaction. Locking the
// upstream writer also fences a failure between upstream and platform recovery.
func FallbackRecoveryCurrent(tx *gorm.DB, a *Attempt) (bool, error) {
	if a == nil {
		return false, nil
	}
	if err := tx.Model(&Guard{}).Where("id = 1").UpdateColumn("id", gorm.Expr("id")).Error; err != nil {
		return false, err
	}
	var states []State
	if err := tx.Where("id IN ?", a.Identity.keys()).Find(&states).Error; err != nil {
		return false, err
	}
	now := time.Now().Unix()
	for _, state := range states {
		version, found := a.Versions[state.ID]
		if recovered, ok := a.Recovered[state.ID]; ok {
			version = recovered
		}
		if !found || state.Version != version || state.OpenUntil != 0 || state.RateLimitedUntil > now {
			return false, nil
		}
	}
	return true, nil
}

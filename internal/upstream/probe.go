package upstream

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// All recovery and legacy health checks share this durable concurrency budget.
type ProbeSlot struct {
	ID         string `gorm:"primaryKey;size:36"`
	Identity   `gorm:"embedded"`
	LeaseUntil int64 `gorm:"index"`
}

func (ProbeSlot) TableName() string { return "upstream_probe_slots" }

func claimSlot(tx *gorm.DB, identity Identity, except string, p Policy) (string, error) {
	now := time.Now().Unix()
	var states []State
	if err := tx.Where("id IN ? AND open_until <> 0 AND id <> ?", identity.keys(), except).Find(&states).Error; err != nil {
		return "", err
	}
	for _, s := range states {
		// Wider due protections recover first. This prevents circular waits
		// while never bypassing another scope's provider cooldown.
		if except == "" || s.OpenUntil > now || s.Scope == "credential" || s.Scope == "credential_model" && except != identity.key("credential") || s.Scope == "model" && except == identity.key("interface") {
			return "", nil
		}
	}
	var count int64
	if err := tx.Model(&ProbeSlot{}).Where("lease_until > ?", now).Count(&count).Error; err != nil {
		return "", err
	}
	if count >= int64(p.ProbeConcurrency) {
		return "", nil
	}
	if err := tx.Model(&ProbeSlot{}).Where("lease_until > ? AND channel_id = ? AND generation = ? AND (model = ? OR credential = ?)", now, identity.ChannelID, identity.Generation, identity.Model, identity.Credential).Count(&count).Error; err != nil {
		return "", err
	}
	if count != 0 {
		return "", nil
	}
	id := uuid.NewString()
	err := tx.Create(&ProbeSlot{ID: id, Identity: identity, LeaseUntil: now + int64(p.ProbeTimeout) + 10}).Error
	return id, err
}

func ClaimCheck(db *gorm.DB, identity Identity, p Policy) (string, error) {
	var id string
	err := write(db, func(tx *gorm.DB) error { var err error; id, err = claimSlot(tx, identity, "", p); return err })
	return id, err
}

func ReleaseCheck(db *gorm.DB, id string) error { return db.Delete(&ProbeSlot{}, "id = ?", id).Error }

// ProbeAdmitted rechecks the lease and other cooldowns immediately before send.
func ProbeAdmitted(db *gorm.DB, s State) (bool, error) {
	var count int64
	if err := db.Model(&State{}).Where("id = ? AND probe_id = ? AND version = ? AND lease_until > ?", s.ID, s.ProbeID, s.Version, time.Now().Unix()).Count(&count).Error; err != nil || count == 0 {
		return false, err
	}
	err := db.Model(&State{}).Where("id IN ? AND id <> ? AND open_until > ?", s.Identity.keys(), s.ID, time.Now().Unix()).Count(&count).Error
	return count == 0, err
}

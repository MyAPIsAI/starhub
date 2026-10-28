package model

import (
	"errors"

	"gorm.io/gorm"
)

var ErrChannelRevisionUnavailable = errors.New("channel configuration revision is unavailable")

// ChannelConfigurationState survives channel deletion and protects against ABA changes.
// Traffic counters and latency observations do not change the configuration revision.
type ChannelConfigurationState struct {
	ID       int   `gorm:"primaryKey"`
	Revision int64 `gorm:"not null;default:0"`
}

func mutateChannelConfiguration(mutate func(*gorm.DB) error) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		// Take the SQLite writer before reading channel or ability state.
		claim := tx.Model(&ChannelConfigurationState{}).
			Where("id = 1 AND revision >= 0 AND revision < ?", maxResourceRevision).
			UpdateColumn("revision", gorm.Expr("revision + 1"))
		if claim.Error != nil {
			return claim.Error
		}
		if claim.RowsAffected != 1 {
			return ErrChannelRevisionUnavailable
		}
		return mutate(tx)
	})
}

package model

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// BackfillChannelModelMetadata explicitly registers missing models from existing channels.
// It never runs during startup and does not change channel or billing configuration.
func BackfillChannelModelMetadata(ctx context.Context) (int64, error) {
	var inserted int64
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		locked := tx.Model(&ChannelConfigurationState{}).
			Where("id = 1 AND revision >= 0 AND revision <= ?", maxResourceRevision).
			UpdateColumn("revision", gorm.Expr("revision"))
		if locked.Error != nil {
			return locked.Error
		}
		if locked.RowsAffected != 1 {
			return ErrChannelRevisionUnavailable
		}
		var lists []string
		if err := tx.Model(&Channel{}).Order("id").Pluck("models", &lists).Error; err != nil {
			return err
		}
		var names []string
		for _, list := range lists {
			names = append(names, SplitList(list)...)
		}
		var err error
		inserted, err = ensureChannelModelMetadata(tx, names)
		return err
	})
	if err != nil {
		return 0, err
	}
	return inserted, nil
}

func uniqueModelNames(names []string) []string {
	seen := make(map[string]bool, len(names))
	result := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name != "" && !seen[name] {
			seen[name] = true
			result = append(result, name)
		}
	}
	return result
}

// The caller owns the transaction; prices and existing metadata are never replaced.
func ensureChannelModelMetadata(tx *gorm.DB, names []string) (int64, error) {
	names = uniqueModelNames(names)
	if len(names) == 0 {
		return 0, nil
	}
	locked := tx.Model(&ModelMetadataState{}).
		Where("id = 1 AND revision >= 0 AND revision <= ?", maxModelMetadataRevision).
		UpdateColumn("revision", gorm.Expr("revision"))
	if locked.Error != nil {
		return 0, locked.Error
	}
	if locked.RowsAffected != 1 {
		return 0, fmt.Errorf("model metadata revision is unavailable for writing")
	}
	rows := make([]ModelMeta, len(names))
	for i, name := range names {
		rows[i].Model = name
		rows[i].AutoRegistered = true
	}
	inserted := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "model"}}, DoNothing: true}).
		CreateInBatches(&rows, 100)
	if inserted.Error != nil {
		return 0, inserted.Error
	}
	if inserted.RowsAffected == 0 {
		return 0, nil
	}
	updated := tx.Model(&ModelMetadataState{}).
		Where("id = 1 AND revision >= 0 AND revision < ?", maxModelMetadataRevision).
		UpdateColumn("revision", gorm.Expr("revision + 1"))
	if updated.Error != nil {
		return 0, updated.Error
	}
	if updated.RowsAffected != 1 {
		return 0, fmt.Errorf("model metadata revision is unavailable for writing")
	}
	return inserted.RowsAffected, nil
}

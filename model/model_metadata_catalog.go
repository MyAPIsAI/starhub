package model

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

const maxModelMetadataRevision int64 = 1<<53 - 1

var ErrModelMetadataConflict = errors.New("model metadata catalog changed")
var ErrInvalidModelMetadataRevision = errors.New("invalid model metadata revision")

// The catalog revision survives row deletion, preventing same-name recreation
// from satisfying an old editor's precondition. Billing-option versions are separate.
type ModelMetadataState struct {
	ID       int64 `gorm:"primaryKey"`
	Revision int64 `gorm:"not null;default:0"`
}

type ModelMetadataCatalog struct {
	Revision int64       `json:"revision"`
	Models   []ModelMeta `json:"models"`
}

func ReadModelMetadataCatalog(ctx context.Context) (*ModelMetadataCatalog, error) {
	result := &ModelMetadataCatalog{Models: []ModelMeta{}}
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var state ModelMetadataState
		if err := tx.First(&state, 1).Error; err != nil {
			return err
		}
		if state.Revision < 0 || state.Revision > maxModelMetadataRevision {
			return fmt.Errorf("model metadata revision is unavailable")
		}
		result.Revision = state.Revision
		return tx.Order("model").Find(&result.Models).Error
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func UpsertModelMetaIfRevision(ctx context.Context, meta *ModelMeta, expected int64) (int64, error) {
	if expected < 0 || expected >= maxModelMetadataRevision {
		return 0, ErrInvalidModelMetadataRevision
	}
	return saveModelMetadata(ctx, meta, &expected)
}

func saveModelMetadata(ctx context.Context, meta *ModelMeta, expected *int64) (int64, error) {
	if meta == nil {
		return 0, fmt.Errorf("model metadata is required")
	}
	stored := *meta
	revision, err := mutateModelMetadata(ctx, &stored, "", expected)
	if err == nil {
		*meta = stored
	}
	return revision, err
}

func DeleteModelMetaIfRevision(ctx context.Context, name string, expected int64) (int64, error) {
	if expected < 0 || expected >= maxModelMetadataRevision {
		return 0, ErrInvalidModelMetadataRevision
	}
	return mutateModelMetadata(ctx, nil, name, &expected)
}

func mutateModelMetadata(ctx context.Context, meta *ModelMeta, removed string, expected *int64) (int64, error) {
	var revision int64
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		locked := tx.Model(&ModelMetadataState{}).Where("id = 1 AND revision >= 0 AND revision < ?", maxModelMetadataRevision).
			UpdateColumn("revision", gorm.Expr("revision"))
		if locked.Error != nil {
			return locked.Error
		}
		if locked.RowsAffected != 1 {
			return fmt.Errorf("model metadata revision is unavailable for writing")
		}
		var state ModelMetadataState
		if err := tx.First(&state, 1).Error; err != nil {
			return err
		}
		if expected != nil && state.Revision != *expected {
			return ErrModelMetadataConflict
		}
		if meta != nil {
			if err := tx.Save(meta).Error; err != nil {
				return err
			}
		} else {
			deleted := tx.Where("model = ?", removed).Delete(&ModelMeta{})
			if deleted.Error != nil {
				return deleted.Error
			}
			if deleted.RowsAffected == 0 {
				revision = state.Revision
				return nil
			}
		}
		updated := tx.Model(&ModelMetadataState{}).Where("id = 1 AND revision = ?", state.Revision).
			UpdateColumn("revision", state.Revision+1)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return ErrModelMetadataConflict
		}
		revision = state.Revision + 1
		return nil
	})
	if err != nil {
		return 0, err
	}
	return revision, nil
}

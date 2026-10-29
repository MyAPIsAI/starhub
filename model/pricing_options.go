package model

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"gorm.io/gorm"

	"starhub/setting"
)

var ErrInvalidPricingOptions = errors.New("invalid pricing options or version precondition")

var pricingOptionKeys = []string{
	setting.OptionKeyModelRatio, setting.OptionKeyCompletionRatio,
	setting.OptionKeyModelPrice, setting.OptionKeyModelPriceExpr,
	setting.OptionKeyCacheRatio, setting.OptionKeyAudioRatio,
}

const maxPricingOptionBytes = 1 << 20
const maxPricingOptionVersion int64 = 1<<53 - 1

type PricingOptions struct {
	Version int64    `json:"version"`
	Options []Option `json:"options"`
}

func isPricingOption(key string) bool {
	for _, allowed := range pricingOptionKeys {
		if key == allowed {
			return true
		}
	}
	return false
}

// ReadPricingOptions returns stored values and their version from one snapshot.
// Validation never replaces missing or corrupt storage with compiled defaults.
func ReadPricingOptions(ctx context.Context) (*PricingOptions, error) {
	result := &PricingOptions{}
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var revision OptionRevision
		if err := tx.First(&revision, 1).Error; err != nil {
			return err
		}
		if revision.Version < 0 || revision.Version > maxPricingOptionVersion {
			return fmt.Errorf("pricing option version is unavailable")
		}
		result.Version = revision.Version
		if err := tx.Where("`key` IN ?", pricingOptionKeys).Order("`key`").Find(&result.Options).Error; err != nil {
			return err
		}
		if len(result.Options) != len(pricingOptionKeys) {
			return fmt.Errorf("pricing options are incomplete")
		}
		for _, option := range result.Options {
			if _, err := setting.NormalizeOption(option.Key, option.Value); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// SavePricingOptionsIfVersion commits a bounded, validated set under one writer
// lock. Each changed key retains its existing per-key history version; readers
// cannot observe a partially committed pricing pair. Other keys are untouched.
func SavePricingOptionsIfVersion(ctx context.Context, options []Option, actorID, expectedVersion int64) (int64, error) {
	if len(options) == 0 || len(options) > len(pricingOptionKeys) || expectedVersion < 0 || expectedVersion > maxPricingOptionVersion-int64(len(options)) {
		return 0, ErrInvalidPricingOptions
	}
	normalized := make([]Option, 0, len(options))
	seen := make(map[string]bool, len(options))
	size := 0
	rawSize := 0
	for _, option := range options {
		rawSize += len(option.Value)
		if rawSize > maxPricingOptionBytes {
			return 0, ErrInvalidPricingOptions
		}
		if !isPricingOption(option.Key) || seen[option.Key] {
			return 0, ErrInvalidPricingOptions
		}
		seen[option.Key] = true
		value, err := setting.NormalizeOption(option.Key, option.Value)
		if err != nil {
			return 0, fmt.Errorf("%w: %s", ErrInvalidPricingOptions, option.Key)
		}
		size += len(value)
		if size > maxPricingOptionBytes {
			return 0, ErrInvalidPricingOptions
		}
		normalized = append(normalized, Option{Key: option.Key, Value: value})
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i].Key < normalized[j].Key })
	var version int64
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		locked := tx.Model(&OptionRevision{}).Where("id = 1").UpdateColumn("version", gorm.Expr("version"))
		if locked.Error != nil {
			return locked.Error
		}
		if locked.RowsAffected != 1 {
			return fmt.Errorf("option revision is unavailable")
		}
		var revision OptionRevision
		if err := tx.First(&revision, 1).Error; err != nil {
			return err
		}
		if revision.Version != expectedVersion {
			return ErrOptionVersionConflict
		}
		version = revision.Version
		for _, option := range normalized {
			var err error
			version, err = saveOptionTx(tx, option.Key, option.Value, actorID, -1)
			if err != nil {
				if errors.Is(err, setting.ErrTokenPriceMode) {
					return fmt.Errorf("%w: %v", ErrInvalidPricingOptions, err)
				}
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return version, nil
}

package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"starhub/pkg/pricingtime"
	"starhub/setting"
)

var ErrInvalidTimePricing = errors.New("invalid time-pricing rules or version precondition")

type TimePricingConfiguration struct {
	Version int64           `json:"version"`
	Rules   json.RawMessage `json:"rules"`
}

func ReadTimePricing(ctx context.Context) (*TimePricingConfiguration, error) {
	result := &TimePricingConfiguration{}
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var revision OptionRevision
		if err := tx.First(&revision, 1).Error; err != nil {
			return err
		}
		if revision.Version < 0 || revision.Version > maxPricingOptionVersion {
			return fmt.Errorf("time-pricing version is unavailable")
		}
		var option Option
		if err := tx.Where("`key` = ?", setting.OptionKeyModelTimePricing).First(&option).Error; err != nil {
			return err
		}
		if _, err := pricingtime.Decode(option.Value); err != nil {
			return err
		}
		result.Version = revision.Version
		result.Rules = json.RawMessage(option.Value)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// SaveTimePricingIfVersion takes the SQLite writer before comparing the shared
// option revision, so independent price editors invalidate stale time drafts.
func SaveTimePricingIfVersion(ctx context.Context, raw string, actorID, expectedVersion int64) (int64, error) {
	if expectedVersion < 0 || expectedVersion >= maxPricingOptionVersion {
		return 0, ErrInvalidTimePricing
	}
	catalog, err := pricingtime.Decode(raw)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrInvalidTimePricing, err)
	}
	var version int64
	err = DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
		var err error
		version, err = saveOptionTx(tx, setting.OptionKeyModelTimePricing, catalog.JSON(), actorID, -1)
		return err
	})
	if err != nil {
		return 0, err
	}
	return version, nil
}

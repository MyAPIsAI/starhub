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

var ErrInvalidTokenTimePricing = errors.New("invalid independent token-pricing rules or version precondition")

func ReadTokenTimePricing(ctx context.Context) (*TimePricingConfiguration, error) {
	result := &TimePricingConfiguration{}
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var revision OptionRevision
		if err := tx.First(&revision, 1).Error; err != nil {
			return err
		}
		if revision.Version < 0 || revision.Version > maxPricingOptionVersion {
			return fmt.Errorf("token-pricing version is unavailable")
		}
		var option Option
		if err := tx.Where("`key` = ?", setting.OptionKeyModelTimeTokenPrices).First(&option).Error; err != nil {
			return err
		}
		if _, err := pricingtime.DecodeTokenPrices(option.Value); err != nil {
			return err
		}
		result.Version, result.Rules = revision.Version, json.RawMessage(option.Value)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func SaveTokenTimePricingIfVersion(ctx context.Context, raw string, actorID, expectedVersion int64) (int64, error) {
	if expectedVersion < 0 || expectedVersion >= maxPricingOptionVersion {
		return 0, ErrInvalidTokenTimePricing
	}
	catalog, err := pricingtime.DecodeTokenPrices(raw)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrInvalidTokenTimePricing, err)
	}
	var version int64
	err = DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		version, err = saveOptionTx(tx, setting.OptionKeyModelTimeTokenPrices, catalog.JSON(), actorID, expectedVersion)
		return err
	})
	if err != nil {
		if errors.Is(err, setting.ErrTokenPriceMode) {
			return 0, fmt.Errorf("%w: %v", ErrInvalidTokenTimePricing, err)
		}
		return 0, err
	}
	return version, nil
}

// Every writer (including history rollback and the generic option endpoint)
// validates the coupled billing modes under the same SQLite writer lock.
func lockTokenPriceOptionTx(tx *gorm.DB, key string) error {
	if key != setting.OptionKeyModelTimeTokenPrices && key != setting.OptionKeyModelPrice && key != setting.OptionKeyModelPriceExpr {
		return nil
	}
	locked := tx.Model(&OptionRevision{}).Where("id = 1").UpdateColumn("version", gorm.Expr("version"))
	if locked.Error != nil {
		return locked.Error
	}
	if locked.RowsAffected != 1 {
		return fmt.Errorf("option revision is unavailable")
	}
	return nil
}

func validateTokenPriceOptionTx(tx *gorm.DB, key, value string) error {
	if key != setting.OptionKeyModelTimeTokenPrices && key != setting.OptionKeyModelPrice && key != setting.OptionKeyModelPriceExpr {
		return nil
	}
	keys := []string{setting.OptionKeyModelTimeTokenPrices, setting.OptionKeyModelPrice, setting.OptionKeyModelPriceExpr}
	var options []Option
	if err := tx.Where("`key` IN ?", keys).Find(&options).Error; err != nil {
		return err
	}
	values := map[string]string{}
	for _, item := range keys {
		values[item] = "{}"
	}
	for _, option := range options {
		values[option.Key] = option.Value
	}
	values[key] = value
	return setting.ValidateTokenPriceModes(values)
}

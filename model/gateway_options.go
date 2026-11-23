package model

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"starhub/config"
)

var ErrInvalidGatewayOptions = errors.New("invalid gateway limits or version")

type GatewayOptions struct {
	Version int64          `json:"version"`
	Options []Option       `json:"options"`
	History []OptionChange `json:"history"`
}

func ReadGatewayOptions(ctx context.Context) (*GatewayOptions, error) {
	result := &GatewayOptions{History: []OptionChange{}}
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var revision OptionRevision
		if err := tx.First(&revision, 1).Error; err != nil {
			return err
		}
		result.Version = revision.Version
		if revision.Version < 0 || revision.Version > 1<<53-1 {
			return ErrInvalidGatewayOptions
		}
		keys := config.GatewayOptionKeys()
		if err := tx.Where(productionQuote(tx, "key")+" IN ?", keys).Find(&result.Options).Error; err != nil {
			return err
		}
		if len(result.Options) != len(keys) {
			return fmt.Errorf("gateway settings are incomplete")
		}
		for _, option := range result.Options {
			if _, err := config.NormalizeOption(option.Key, option.Value); err != nil {
				return err
			}
		}
		if err := tx.Where(productionQuote(tx, "key")+" IN ?", keys).Order("version DESC").Limit(20).Find(&result.History).Error; err != nil {
			return err
		}
		if err := tx.First(&revision, 1).Error; err != nil {
			return err
		}
		if revision.Version != result.Version {
			return ErrOptionVersionConflict
		}
		return nil
	})
	return result, err
}

// Keep the existing per-key history while committing all four limits together.
func SaveGatewayOptions(ctx context.Context, options []Option, actorID, expected int64) (int64, error) {
	if len(options) != len(config.GatewayOptionKeys()) || expected < 0 || expected > (1<<53-1)-4 {
		return 0, ErrInvalidGatewayOptions
	}
	values := make(map[string]string, len(options))
	for _, option := range options {
		if !config.IsGatewayOption(option.Key) {
			return 0, ErrInvalidGatewayOptions
		}
		if _, found := values[option.Key]; found {
			return 0, ErrInvalidGatewayOptions
		}
		normalized, err := config.NormalizeOption(option.Key, option.Value)
		if err != nil {
			return 0, fmt.Errorf("%w: %s", ErrInvalidGatewayOptions, option.Key)
		}
		values[option.Key] = normalized
	}
	version := expected
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, key := range config.GatewayOptionKeys() {
			var err error
			version, err = saveOptionTx(tx, key, values[key], actorID, version)
			if err != nil {
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

package model

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"starhub/setting"
)

var ErrInvalidAffinityBatch = errors.New("invalid global affinity settings")

// SaveAffinityOptionsIfVersion preserves per-option history but commits the form as a unit.
func SaveAffinityOptionsIfVersion(ctx context.Context, options []Option, actorID, expected int64) (int64, error) {
	if len(options) == 0 || len(options) > 5 || expected < 0 || expected > (1<<53-1)-int64(len(options)) {
		return 0, ErrInvalidAffinityBatch
	}
	seen := make(map[string]bool, len(options))
	for _, o := range options {
		switch o.Key {
		case setting.OptionKeyRoutingAffinityEnabled, setting.OptionKeyRoutingAffinityMode, setting.OptionKeyRoutingAffinityTTL, setting.OptionKeyRoutingAffinityCooldown, setting.OptionKeyRoutingAffinityPrefixCache:
		default:
			return 0, ErrInvalidAffinityBatch
		}
		v, err := setting.NormalizeOption(o.Key, o.Value)
		// Reject clamped numbers rather than storing a value different from the form.
		if seen[o.Key] || err != nil || v != o.Value {
			return 0, fmt.Errorf("%w: %s", ErrInvalidAffinityBatch, o.Key)
		}
		seen[o.Key] = true
	}
	version := expected
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, o := range options {
			var err error
			version, err = saveOptionTx(tx, o.Key, o.Value, actorID, version)
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

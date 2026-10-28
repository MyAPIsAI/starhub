package model

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"

	"gorm.io/gorm"

	"starhub/setting"
)

// SaveServiceGroupAndRatio commits metadata and its billing-option change together.
// Settings are reloaded separately after commit by the serving controller.
func SaveServiceGroupAndRatio(ctx context.Context, group *ServiceGroupConfig, actorID int64) error {
	if group == nil {
		return fmt.Errorf("service group is required")
	}
	stored := *group
	if stored.Multiplier == "" {
		stored.Multiplier = "1"
	}
	value, err := strconv.ParseFloat(stored.Multiplier, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 || value > 100 {
		return fmt.Errorf("invalid service group multiplier")
	}
	if err := mutateServiceGroupAndRatio(ctx, &stored, "", actorID); err != nil {
		return err
	}
	*group = stored
	return nil
}

func DeleteServiceGroupAndRatio(ctx context.Context, groupID string, actorID int64) error {
	return mutateServiceGroupAndRatio(ctx, nil, groupID, actorID)
}

func mutateServiceGroupAndRatio(ctx context.Context, group *ServiceGroupConfig, removed string, actorID int64) error {
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Acquire the SQLite writer before reading either metadata or the option.
		locked := tx.Model(&OptionRevision{}).Where("id = 1").UpdateColumn("version", gorm.Expr("version"))
		if locked.Error != nil {
			return locked.Error
		}
		if locked.RowsAffected != 1 {
			return fmt.Errorf("option revision is unavailable")
		}
		if group != nil {
			if err := upsertServiceGroup(tx, group); err != nil {
				return err
			}
		} else if err := deleteServiceGroup(tx, removed); err != nil {
			return err
		}

		// Change only the requested group, preserving legacy and independently edited ratios.
		ratios := map[string]float64{"default": 1}
		var option Option
		err := tx.Where("`key` = ?", setting.OptionKeyGroupRatio).First(&option).Error
		if err != nil && err != gorm.ErrRecordNotFound {
			return err
		}
		if err == nil {
			ratios = nil
			if err := json.Unmarshal([]byte(option.Value), &ratios); err != nil {
				return err
			}
			if ratios == nil {
				ratios = map[string]float64{}
			}
		}
		if removed != "" {
			delete(ratios, removed)
		}
		if group != nil {
			if group.Enabled {
				// The public save entry validates this exact stored value before starting the transaction.
				value, err := strconv.ParseFloat(group.Multiplier, 64)
				if err != nil {
					return err
				}
				ratios[group.GroupID] = value
			} else {
				delete(ratios, group.GroupID)
			}
		}
		encoded, err := json.Marshal(ratios)
		if err != nil {
			return err
		}
		_, err = saveOptionTx(tx, setting.OptionKeyGroupRatio, string(encoded), actorID, -1)
		return err
	})
}

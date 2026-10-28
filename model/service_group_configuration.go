package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"starhub/setting"
)

// Metadata revisions are separate from SLA targets and global pricing-option versions.
const maxServiceGroupRevision int64 = 1<<53 - 1

var ErrServiceGroupConflict = errors.New("service group identity or revision changed")
var ErrInvalidServiceGroupPrecondition = errors.New("invalid service group precondition")

type serviceGroupPrecondition struct {
	id, revision  int64
	optionVersion *int64
}

func validServiceGroupPrecondition(id, revision int64) bool {
	return id >= 0 && id <= maxServiceGroupRevision && revision >= 0 && revision < maxServiceGroupRevision && (id != 0 || revision == 0)
}

func SaveServiceGroupAndRatioIfRevision(ctx context.Context, group *ServiceGroupConfig, actorID, expectedID, expectedRevision int64) error {
	if !validServiceGroupPrecondition(expectedID, expectedRevision) {
		return ErrInvalidServiceGroupPrecondition
	}
	return saveServiceGroupAndRatio(ctx, group, actorID, &serviceGroupPrecondition{id: expectedID, revision: expectedRevision})
}

func SaveServiceGroupAndRatioIfState(ctx context.Context, group *ServiceGroupConfig, actorID, expectedID, expectedRevision, expectedOptionVersion int64) error {
	if !validServiceGroupPrecondition(expectedID, expectedRevision) || expectedOptionVersion < 0 || expectedOptionVersion >= maxServiceGroupRevision {
		return ErrInvalidServiceGroupPrecondition
	}
	return saveServiceGroupAndRatio(ctx, group, actorID, &serviceGroupPrecondition{id: expectedID, revision: expectedRevision, optionVersion: &expectedOptionVersion})
}

// SaveServiceGroupAndRatio commits metadata and its billing-option change together.
// Settings are reloaded separately after commit by the serving controller.
func SaveServiceGroupAndRatio(ctx context.Context, group *ServiceGroupConfig, actorID int64) error {
	return saveServiceGroupAndRatio(ctx, group, actorID, nil)
}

func saveServiceGroupAndRatio(ctx context.Context, group *ServiceGroupConfig, actorID int64, expected *serviceGroupPrecondition) error {
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
	if err := mutateServiceGroupAndRatio(ctx, &stored, "", actorID, expected); err != nil {
		return err
	}
	*group = stored
	return nil
}

func DeleteServiceGroupAndRatio(ctx context.Context, groupID string, actorID int64) error {
	return mutateServiceGroupAndRatio(ctx, nil, groupID, actorID, nil)
}

func DeleteServiceGroupAndRatioIfRevision(ctx context.Context, groupID string, actorID, expectedID, expectedRevision int64) error {
	if expectedID == 0 || !validServiceGroupPrecondition(expectedID, expectedRevision) {
		return ErrInvalidServiceGroupPrecondition
	}
	return mutateServiceGroupAndRatio(ctx, nil, groupID, actorID, &serviceGroupPrecondition{id: expectedID, revision: expectedRevision})
}

func DeleteServiceGroupAndRatioIfState(ctx context.Context, groupID string, actorID, expectedID, expectedRevision, expectedOptionVersion int64) error {
	if expectedID <= 0 || !validServiceGroupPrecondition(expectedID, expectedRevision) || expectedOptionVersion < 0 || expectedOptionVersion >= maxServiceGroupRevision {
		return ErrInvalidServiceGroupPrecondition
	}
	return mutateServiceGroupAndRatio(ctx, nil, groupID, actorID, &serviceGroupPrecondition{id: expectedID, revision: expectedRevision, optionVersion: &expectedOptionVersion})
}

func mutateServiceGroupAndRatio(ctx context.Context, group *ServiceGroupConfig, removed string, actorID int64, expected *serviceGroupPrecondition) error {
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Acquire the SQLite writer before reading either metadata or the option.
		locked := tx.Model(&OptionRevision{}).Where("id = 1").UpdateColumn("version", gorm.Expr("version"))
		if locked.Error != nil {
			return locked.Error
		}
		if locked.RowsAffected != 1 {
			return fmt.Errorf("option revision is unavailable")
		}
		if expected != nil {
			if expected.optionVersion != nil {
				var revision OptionRevision
				if err := tx.First(&revision, 1).Error; err != nil {
					return err
				}
				if revision.Version != *expected.optionVersion {
					return ErrOptionVersionConflict
				}
			}
			id := removed
			if group != nil {
				id = strings.TrimSpace(group.GroupID)
			}
			var current ServiceGroupConfig
			err := tx.Where("group_id = ?", id).First(&current).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if errors.Is(err, gorm.ErrRecordNotFound) {
				if expected.id != 0 {
					return ErrServiceGroupNotFound
				}
			} else if expected.id == 0 || current.ID != expected.id || current.Revision != expected.revision {
				return ErrServiceGroupConflict
			}
		}
		if group != nil {
			if err := validateServiceGroupAccounts(tx, group); err != nil {
				return err
			}
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

package model

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

var (
	ErrInvalidSLATarget  = errors.New("invalid service-level target")
	ErrSLATargetConflict = errors.New("service group or target revision changed")
)

const maxSLARevision int64 = 1<<53 - 1

// Targets are operator-published metadata, separate from observed quality or billing.
// Nil means unpublished; a published zero success target is distinct from nil.
type ServiceGroupSLATarget struct {
	SuccessRatePPM *int64 `json:"success_rate_ppm"`
	P95LatencyMS   *int64 `json:"p95_latency_ms"`
}

type ServiceGroupSLA struct {
	Revision  int64                  `json:"revision"`
	Targets   *ServiceGroupSLATarget `json:"targets"`
	UpdatedAt int64                  `json:"updated_at"`
}

func (group *ServiceGroupConfig) ServiceLevel() ServiceGroupSLA {
	result := ServiceGroupSLA{Revision: group.SLARevision, UpdatedAt: group.SLAUpdatedAt}
	if group.SLASuccessPPM != nil || group.SLAP95MS != nil {
		result.Targets = &ServiceGroupSLATarget{SuccessRatePPM: group.SLASuccessPPM, P95LatencyMS: group.SLAP95MS}
	}
	return result
}

// PublishedServiceGroupSLA returns only enabled, operator-published group metadata.
// Missing definitions remain unknown and are never filled from product examples.
func PublishedServiceGroupSLA(ctx context.Context, groupID string) (*ServiceGroupSLA, error) {
	var group ServiceGroupConfig
	if err := DB.WithContext(ctx).Where("group_id = ?", groupID).First(&group).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if !group.Enabled {
		return nil, nil
	}
	state := group.ServiceLevel()
	if state.Revision < 0 || state.Revision > maxSLARevision || state.UpdatedAt < 0 {
		return nil, ErrInvalidSLATarget
	}
	if err := validateSLATarget(state.Targets); err != nil {
		return nil, err
	}
	if state.Targets != nil && (state.Revision < 1 || state.UpdatedAt < 1) {
		return nil, ErrInvalidSLATarget
	}
	return &state, nil
}

func validateSLATarget(target *ServiceGroupSLATarget) error {
	if target == nil {
		return nil
	}
	if target.SuccessRatePPM == nil && target.P95LatencyMS == nil {
		return ErrInvalidSLATarget
	}
	if target.SuccessRatePPM != nil && (*target.SuccessRatePPM < 0 || *target.SuccessRatePPM > 1_000_000) {
		return ErrInvalidSLATarget
	}
	if target.P95LatencyMS != nil && (*target.P95LatencyMS < 1 || *target.P95LatencyMS > 86_400_000) {
		return ErrInvalidSLATarget
	}
	return nil
}

// UpdateServiceGroupSLA replaces the target tuple with a guarded, atomic write.
// Row identity also participates so a deleted/recreated group cannot accept an old editor.
func UpdateServiceGroupSLA(ctx context.Context, groupID string, expectedID, expectedRevision int64, target *ServiceGroupSLATarget) (*ServiceGroupConfig, error) {
	if groupID == "" || expectedID < 1 || expectedID > maxSLARevision || expectedRevision < 0 || expectedRevision >= maxSLARevision {
		return nil, ErrInvalidSLATarget
	}
	if err := validateSLATarget(target); err != nil {
		return nil, err
	}
	var success, p95 *int64
	if target != nil {
		success, p95 = target.SuccessRatePPM, target.P95LatencyMS
	}
	var result ServiceGroupConfig
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		update := tx.Model(&ServiceGroupConfig{}).Where("id = ? AND group_id = ? AND sla_revision = ?", expectedID, groupID, expectedRevision).Updates(map[string]any{"sla_success_ppm": success, "sla_p95_ms": p95, "sla_revision": expectedRevision + 1, "sla_updated_at": time.Now().Unix()})
		if update.Error != nil {
			return update.Error
		}
		if err := tx.Where("group_id = ?", groupID).First(&result).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrServiceGroupNotFound
			}
			return err
		}
		if update.RowsAffected != 1 {
			return ErrSLATargetConflict
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

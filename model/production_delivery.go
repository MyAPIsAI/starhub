package model

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"time"
)

// Select metadata first so a full row batch cannot transfer many MiB of
// payloads over a database connection before applying the byte budget.
func DueProductionAttempts(ctx context.Context, now time.Time) ([]ProductionAttemptRecord, error) {
	db := DB.WithContext(ctx)
	var candidates []ProductionAttemptRecord
	columns := "id,minute,next_attempt_at," + productionPayloadLength(db, "payload") + " AS payload_bytes"
	if err := PublishedProductionAttempts(db).Select(columns).Where("delivered_at = 0 AND next_attempt_at <= ?", now.Unix()).Order("next_attempt_at,minute,id").Limit(64).Find(&candidates).Error; err != nil {
		return nil, err
	}
	ids := []string{}
	bytes := int64(0)
	for _, row := range candidates {
		if row.PayloadBytes > 128<<10 {
			return nil, errors.Join(ErrProductionEvidence, errors.New("attempt payload exceeds delivery budget"))
		}
		if bytes+row.PayloadBytes+1024 > 1<<20 {
			break
		}
		ids = append(ids, row.ID)
		bytes += row.PayloadBytes + 1024
	}
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []ProductionAttemptRecord
	err := PublishedProductionAttempts(db).Where("id IN ? AND delivered_at = 0", ids).Order("next_attempt_at,minute,id").Find(&rows).Error
	return rows, err
}

// CompleteProductionDelivery updates one receipt batch in a bounded transaction.
// Retry groups share an update rather than committing once per attempt.
func CompleteProductionDelivery(rows []ProductionAttemptRecord, failure string, now time.Time) error {
	return CompleteProductionDeliveryContext(context.Background(), rows, failure, now)
}

func CompleteProductionDeliveryContext(ctx context.Context, rows []ProductionAttemptRecord, failure string, now time.Time) error {
	if text := []rune(failure); len(text) > 160 {
		failure = string(text[:160])
	}
	groups := map[int64][]string{}
	for _, row := range rows {
		next := int64(0)
		if failure != "" {
			next = now.Add(ResourceEventRetryDelay(row.Attempts + 1)).Unix()
		}
		groups[next] = append(groups[next], row.ID)
	}
	return productionTransaction(DB.WithContext(ctx), func(tx *gorm.DB) error {
		if err := lockProductionStorage(tx); err != nil {
			return err
		}
		for next, ids := range groups {
			delivered := now.Unix()
			if failure != "" {
				delivered = 0
			}
			if err := tx.Model(&ProductionAttemptRecord{}).Where("id IN ? AND delivered_at = 0", ids).Updates(map[string]any{"attempts": gorm.Expr("attempts + 1"), "delivered_at": delivered, "last_error": failure, "next_attempt_at": next}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

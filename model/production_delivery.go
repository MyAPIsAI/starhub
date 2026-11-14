package model

import (
	"context"
	"gorm.io/gorm"
	"time"
)

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

package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"log"
	"net/http"
	"starhub/setting"
	"time"
)

type ReconciliationAlert struct {
	ID          int64  `gorm:"primaryKey" json:"id"`
	UserID      int64  `json:"user_id"`
	Day         string `json:"day"`
	Payload     string `gorm:"type:text" json:"-"`
	CreatedAt   int64  `json:"created_at"`
	DeliveredAt int64  `gorm:"index;not null;default:0" json:"delivered_at"`
	NextAttempt int64  `gorm:"not null;default:0" json:"next_attempt"`
	Attempts    int    `gorm:"not null;default:0" json:"attempts"`
}

func queueReconciliationAlert(tx *gorm.DB, r ReconciliationReport, now time.Time) error {
	raw, err := json.Marshal(map[string]any{"type": "reconciliation_difference", "severity": "serious", "user_id": r.UserID, "date": r.Date, "timezone": r.Timezone, "differences": r.Differences})
	if err != nil {
		return err
	}
	legacy := ReconciliationAlert{UserID: r.UserID, Day: r.Date, Payload: string(raw), CreatedAt: now.Unix()}
	if err := tx.Create(&legacy).Error; err != nil {
		return err
	}
	scope := fmt.Sprintf("%d:%s", r.UserID, r.Date)
	if err := alertCondition(tx, "reconciliation_difference", scope, "serious", true, float64(len(r.Differences)), 0, now); err != nil {
		return err
	}
	var alert OperationalAlert
	if err := tx.Where("rule = ? AND scope = ?", "reconciliation_difference", scope).First(&alert).Error; err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{"alert_id": alert.ID, "rule": "reconciliation_difference", "type": "reconciliation_difference", "severity": "serious", "scope": scope, "transition": "firing", "occurred_at": now.Unix()})
	return tx.Create(&AlertDelivery{ID: fmt.Sprintf("reconciliation-%d", legacy.ID), AlertID: alert.ID, Channel: "general", Severity: "serious", Transition: "firing", Payload: string(payload), CreatedAt: now.Unix(), ReconciliationID: legacy.ID, Generation: alert.Generation}).Error
}

// ImportLegacyReconciliationAlerts preserves undelivered notices across an upgrade.
func ImportLegacyReconciliationAlerts(ctx context.Context) error {
	var rows []ReconciliationAlert
	if err := DB.WithContext(ctx).Where("delivered_at = 0 AND NOT EXISTS (SELECT 1 FROM alert_deliveries d WHERE d.reconciliation_id = reconciliation_alerts.id)").Order("id").Limit(100).Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		if err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			scope := fmt.Sprintf("%d:%s", row.UserID, row.Day)
			var alert OperationalAlert
			result := tx.Where("rule = ? AND scope = ?", "reconciliation_difference", scope).Limit(1).Find(&alert)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				if err := alertCondition(tx, "reconciliation_difference", scope, "serious", true, 1, 0, time.Unix(row.CreatedAt, 0)); err != nil {
					return err
				}
				if err := tx.Where("rule = ? AND scope = ?", "reconciliation_difference", scope).First(&alert).Error; err != nil {
					return err
				}
			}
			payload, _ := json.Marshal(map[string]any{"alert_id": alert.ID, "rule": "reconciliation_difference", "severity": "serious", "scope": scope, "transition": "firing", "occurred_at": row.CreatedAt})
			delivery := AlertDelivery{ID: fmt.Sprintf("reconciliation-%d", row.ID), AlertID: alert.ID, Channel: "general", Severity: "serious", Transition: "firing", Payload: string(payload), CreatedAt: row.CreatedAt, ReconciliationID: row.ID, Generation: 1}
			return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&delivery).Error
		}); err != nil {
			return err
		}
	}
	return nil
}

// DeliverReconciliationAlerts retries durable notifications after restarts.
// The receiver can deduplicate at-least-once delivery using Idempotency-Key.
func DeliverReconciliationAlerts(ctx context.Context, now time.Time) error {
	var alerts []ReconciliationAlert
	if err := DB.WithContext(ctx).Where("delivered_at = 0 AND next_attempt <= ?", now.Unix()).Order("id").Limit(100).Find(&alerts).Error; err != nil {
		return err
	}
	for _, a := range alerts {
		claimed := DB.WithContext(ctx).Model(&ReconciliationAlert{}).Where("id = ? AND delivered_at = 0 AND next_attempt <= ?", a.ID, now.Unix()).Updates(map[string]any{"next_attempt": now.Add(time.Minute).Unix(), "attempts": gorm.Expr("attempts + 1")})
		if claimed.Error != nil {
			return claimed.Error
		}
		if claimed.RowsAffected == 0 {
			continue
		}
		if a.Attempts == 0 {
			log.Printf("ALERT reconciliation_difference severity=serious user_id=%d day=%s alert_id=%d", a.UserID, a.Day, a.ID)
		}
		endpoint := setting.AlertWebhookURLValue()
		if endpoint == "" {
			continue
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBufferString(a.Payload))
		if err != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", fmt.Sprintf("reconciliation-%d", a.ID))
		client := http.Client{Timeout: 5 * time.Second}
		response, err := client.Do(req)
		if err != nil {
			continue
		}
		response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			continue
		}
		if err := DB.WithContext(ctx).Model(&ReconciliationAlert{}).Where("id = ?", a.ID).Update("delivered_at", now.Unix()).Error; err != nil {
			return err
		}
	}
	return nil
}

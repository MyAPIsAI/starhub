package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
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
	return tx.Create(&ReconciliationAlert{UserID: r.UserID, Day: r.Date, Payload: string(raw), CreatedAt: now.Unix()}).Error
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

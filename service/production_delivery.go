package service

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"time"

	"starhub/model"
)

// DeliverProductionAttemptsOnce retries metadata-only batches independently of
// relay handling and minute sealing. Stable attempt IDs fence duplicate delivery.
func DeliverProductionAttemptsOnce(ctx context.Context) error {
	var rows []model.ProductionAttemptRecord
	if err := model.DB.Where("delivered_at = 0 AND next_attempt_at <= ?", time.Now().Unix()).Order("minute,id").Limit(64).Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	items := []json.RawMessage{}
	for _, row := range rows {
		items = append(items, json.RawMessage(row.Payload))
	}
	payload := map[string]any{"schema": 1, "source": resourceTelemetrySource(), "instance_id": os.Getenv("NODE_NAME"), "attempts": items}
	var ack struct {
		AcceptedIDs []string `json:"accepted_ids"`
	}
	err := postResource(ctx, "/internal/routing/attempt-metadata", payload, &ack)
	if err == nil {
		if len(ack.AcceptedIDs) != len(rows) {
			err = errors.New("attempt acknowledgement mismatch")
		} else {
			for i, row := range rows {
				if ack.AcceptedIDs[i] != row.ID {
					err = errors.New("attempt acknowledgement mismatch")
					break
				}
			}
		}
	}
	for _, row := range rows {
		updates := map[string]any{"attempts": row.Attempts + 1, "last_error": "", "delivered_at": time.Now().Unix()}
		if err != nil {
			updates["delivered_at"] = 0
			updates["last_error"] = err.Error()
			updates["next_attempt_at"] = time.Now().Add(model.ResourceEventRetryDelay(row.Attempts + 1)).Unix()
		}
		if e := model.DB.Model(&model.ProductionAttemptRecord{}).Where("id = ? AND delivered_at = 0", row.ID).Updates(updates).Error; e != nil {
			return e
		}
	}
	return err
}

func resourceTelemetrySource() string {
	if source := os.Getenv("ROUTING_PROTECTION_SOURCE"); source != "" {
		return source
	}
	return "talen"
}

func RunProductionAttemptDelivery(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if os.Getenv("RESOURCE_CATALOG_NOTIFY_URL") != "" {
				if err := DeliverProductionAttemptsOnce(ctx); err != nil {
					log.Printf("attempt metadata delivery failed: %v", err)
				}
			}
		}
	}
}

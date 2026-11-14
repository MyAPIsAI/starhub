package service

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"sync"
	"time"

	"starhub/model"
)

// DeliverProductionAttemptsOnce retries metadata-only batches independently of
// relay handling and minute sealing. Stable attempt IDs fence duplicate delivery.
var productionDeliveryMu sync.Mutex
var productionDeliveryWake = make(chan struct{}, 1)

func DeliverProductionAttemptsOnce(ctx context.Context) error {
	if !productionDeliveryMu.TryLock() {
		return nil
	}
	defer productionDeliveryMu.Unlock()
	budget, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	for batches := 0; batches < 64 && budget.Err() == nil; batches++ {
		count, err := deliverProductionAttemptBatch(budget)
		if err != nil {
			return err
		}
		if count == 0 {
			break
		}
	}
	return ctx.Err()
}

func deliverProductionAttemptBatch(ctx context.Context) (int, error) {
	var rows []model.ProductionAttemptRecord
	if err := model.PublishedProductionAttempts(model.DB).Where("delivered_at = 0 AND next_attempt_at <= ?", time.Now().Unix()).Order("minute,id").Limit(64).Find(&rows).Error; err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
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
	failure := ""
	if err != nil {
		failure = err.Error()
	}
	if e := model.CompleteProductionDelivery(rows, failure, time.Now()); e != nil {
		return 0, e
	}
	return len(rows), err
}

func wakeProductionDelivery() {
	select {
	case productionDeliveryWake <- struct{}{}:
	default:
	}
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
		case <-productionDeliveryWake:
		}
		if os.Getenv("RESOURCE_CATALOG_NOTIFY_URL") != "" {
			if err := DeliverProductionAttemptsOnce(ctx); err != nil {
				log.Printf("attempt metadata delivery failed: %v", err)
			}
		}
	}
}

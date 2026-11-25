package middleware

import (
	"context"
	"log"
	"time"

	"starhub/model"
)

// BeginRoutingDecision times selection only, excluding provider response time.
func BeginRoutingDecision(ctx context.Context) func() {
	observation := RequestObservation(ctx)
	if observation == nil {
		return func() {}
	}
	observation.DecisionCount++
	id := GenerateRequestID()
	started := time.Now()
	return func() {
		if model.DB == nil {
			return
		}
		row := model.EventAuditDecision{ID: id, RequestID: observation.Entry.RequestID, CompletedAt: time.Now().Unix(), DurationUS: time.Since(started).Microseconds()}
		if err := model.DB.Create(&row).Error; err != nil {
			log.Printf("routing decision audit persistence failed: %v", err)
		}
	}
}

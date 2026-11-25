package middleware

import (
	"context"
	"log"
	"time"

	"starhub/model"
)

// BeginRoutingDecision times selection only, excluding provider response time.
func BeginRoutingDecision(ctx context.Context) func(bool) {
	observation := RequestObservation(ctx)
	if observation == nil {
		return func(bool) {}
	}
	observation.DecisionCount++
	id := GenerateRequestID()
	started := time.Now()
	return func(smart bool) {
		if model.DB == nil {
			return
		}
		row := model.EventAuditDecision{ID: id, Smart: smart, RequestID: observation.Entry.RequestID, CompletedAt: time.Now().Unix(), DurationUS: time.Since(started).Microseconds()}
		if err := model.DB.Create(&row).Error; err != nil {
			log.Printf("routing decision audit persistence failed: %v", err)
		}
	}
}

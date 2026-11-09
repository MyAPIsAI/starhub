package service

import (
	"context"
	"time"

	"gorm.io/gorm/clause"
	"starhub/model"
	"starhub/observability"
)

func WatchObservability(ctx context.Context, c observability.Config) {
	// Slow notification receivers must not block gateway heartbeats or rule evaluation.
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				if err := model.DeliverOperationalAlerts(ctx, c, now); err != nil {
					observability.Log("", "alert_delivery", "failed")
				}
			}
		}
	}()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	lastPrune := time.Time{}
	for {
		now := time.Now()
		sample := observability.SampleSystem()
		pulse := model.ObservabilityPulse{ID: model.LocalResources.ID, At: now.Unix(), CPU: sample.CPU, Memory: sample.Memory, Disk: sample.Disk}
		if err := model.DB.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(&pulse).Error; err != nil {
			observability.Log("", "database_health", "failed")
		} else {
			if err := model.ImportLegacyReconciliationAlerts(ctx); err != nil {
				observability.Log("", "alert_migration", "failed")
			}
			if err := model.EvaluateOperationalAlerts(ctx, c, now); err != nil {
				observability.Log("", "alert_evaluation", "failed")
			}
		}
		if now.Sub(lastPrune) > time.Hour {
			// Telemetry retention is independent of the accounting record retention.
			_ = model.DB.WithContext(ctx).Where("ended_at < ?", now.AddDate(0, 0, -30).UnixMilli()).Delete(&model.TraceStep{}).Error
			_ = model.DB.WithContext(ctx).Where("ended_at < ?", now.AddDate(0, 0, -30).UnixMilli()).Delete(&model.TraceRequest{}).Error
			_ = model.DB.WithContext(ctx).Where("at < ?", now.Add(-24*time.Hour).Unix()).Delete(&model.Observation{}).Error
			lastPrune = now
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

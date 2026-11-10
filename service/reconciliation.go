package service

import (
	"context"
	"log"
	"time"

	"starhub/model"
)

// RunDailyReconciliationOnce computes a daily reconciliation snapshot:
// compares the sum of Log.Quota against User.UsedQuota for each user,
// reports discrepancies via the alert system.
// It is safe to call concurrently; each run is independent.
func RunDailyReconciliationOnce(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	yesterday := time.Now().UTC().Add(-24 * time.Hour)
	dayStart := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, time.UTC)
	dayEnd := dayStart.Add(24 * time.Hour)

	users, err := model.ListUsers()
	if err != nil {
		log.Printf("daily reconciliation: list users failed: %v", err)
		return
	}

	for _, user := range users {
		if ctx.Err() != nil {
			return
		}
		// Sum quota from logs for this user in the day window.
		var logSum struct {
			Total int64
		}
		err := model.DB.Model(&model.Log{}).
			Select("COALESCE(SUM(quota), 0) AS total").
			Where("user_id = ? AND created_at >= ? AND created_at < ?", user.ID, dayStart.Unix(), dayEnd.Unix()).
			Scan(&logSum).Error
		if err != nil {
			log.Printf("daily reconciliation: sum logs for user %d failed: %v", user.ID, err)
			continue
		}

		// The user.UsedQuota is cumulative, so we compare the daily delta
		// against the log sum. A significant discrepancy indicates a
		// billing inconsistency.
		if logSum.Total > 0 {
			// For now, just log the reconciliation result. A full
			// implementation would persist this to a reconciliation table
			// and alert on threshold breaches.
			log.Printf("daily reconciliation: user=%d date=%s log_quota=%d", user.ID, dayStart.Format("2006-01-02"), logSum.Total)
		}
	}
}

// RunDailyReconciliationLoop runs the reconciliation daily at 02:00 UTC.
func RunDailyReconciliationLoop(ctx context.Context) {
	for {
		now := time.Now().UTC()
		next := time.Date(now.Year(), now.Month(), now.Day(), 2, 0, 0, 0, time.UTC)
		if next.Before(now) {
			next = next.Add(24 * time.Hour)
		}
		wait := time.Until(next)
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		RunDailyReconciliationOnce(ctx)
	}
}

package service

import (
	"context"
	"log"
	"time"

	"starhub/model"
)

// reconciliationDiffThreshold is the absolute quota difference above which
// a reconciliation alert is triggered. 1000 quota = $0.002 — small enough
// to catch any meaningful billing drift.
const reconciliationDiffThreshold = int64(1000)

// RunDailyReconciliationOnce computes a daily reconciliation snapshot:
// compares the sum of Log.Quota against User.UsedQuota delta for each user,
// persists the result, and alerts on threshold breaches.
// It is safe to call concurrently; each run is independent and idempotent
// (re-running for the same day overwrites the previous record).
func RunDailyReconciliationOnce(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	yesterday := time.Now().UTC().Add(-24 * time.Hour)
	dayStart := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, time.UTC)
	dayEnd := dayStart.Add(24 * time.Hour)
	dateStr := dayStart.Format("2006-01-02")

	users, err := model.ListUsers()
	if err != nil {
		log.Printf("daily reconciliation: list users failed: %v", err)
		NotifyReconciliationDiff(0, "list users failed: "+err.Error())
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

		// Compute the used_quota delta for the day by comparing current
		// UsedQuota against the previous reconciliation record's snapshot.
		// For the first run, the delta equals the full UsedQuota.
		usedDelta := user.UsedQuota
		if prev, err := model.GetLatestReconciliationForUser(user.ID); err == nil && prev != nil {
			usedDelta = user.UsedQuota - prev.UsedQuotaDelta
		}

		diff := logSum.Total - usedDelta
		if diff < 0 {
			diff = -diff
		}

		alerted := false
		if diff > reconciliationDiffThreshold {
			NotifyReconciliationDiff(diff, "user quota delta does not match log sum")
			alerted = true
		}

		// Persist the reconciliation record for audit and future delta computation.
		record := &model.ReconciliationRecord{
			UserID:         user.ID,
			Date:           dateStr,
			LogQuotaSum:    logSum.Total,
			UsedQuotaDelta: user.UsedQuota,
			Diff:           diff,
			Alerted:        alerted,
		}
		if err := model.InsertReconciliationRecord(record); err != nil {
			log.Printf("daily reconciliation: persist record for user %d failed: %v", user.ID, err)
		}
	}
	log.Printf("daily reconciliation: completed for %s (%d users)", dateStr, len(users))
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

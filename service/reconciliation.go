package service

import (
	"context"
	"fmt"
	"log"
	"math"
	"math/big"
	"starhub/common"
	"time"

	"starhub/model"
)

// RunDailyReconciliationOnce revisits settled business days, including historical
// days changed by late receipts. Source-identical reports remain idempotent.
func RunDailyReconciliationOnce(ctx context.Context) {
	users, err := model.ListUsers()
	if err != nil {
		log.Printf("daily reconciliation: list users failed: %v", err)
		return
	}
	for _, user := range users {
		if ctx.Err() != nil {
			return
		}
		days, err := model.ReconciliationDays(user.ID, time.Now())
		if err != nil {
			log.Printf("daily reconciliation: calendar for user %d unavailable: %v", user.ID, err)
			continue
		}
		for _, day := range days {
			if ctx.Err() != nil {
				return
			}
			record, changed, err := model.ReconcileBusinessDay(user.ID, day)
			if err != nil {
				log.Printf("daily reconciliation: user %d day %s failed: %v", user.ID, day.BusinessDate, err)
				continue
			}
			if changed && record.Alerted {
				NotifyReconciliationDiff(record.Diff, fmt.Sprintf("user %d business day %s %s: %s; nano difference %s", user.ID, day.BusinessDate, day.BusinessTimezone, record.State, record.DifferenceNano))
			}
		}
	}
}

// Local 02:00 eligibility is evaluated against each frozen calendar, not UTC.
func RunDailyReconciliationLoop(ctx context.Context) {
	RunDailyReconciliationOnce(ctx)
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			RunDailyReconciliationOnce(ctx)
		}
	}
}

func reconciliationQuotaProjection(amount *big.Int) int64 {
	whole, _, err := common.NanoQuotaParts(amount)
	if err != nil {
		return math.MaxInt64
	}
	return whole
}

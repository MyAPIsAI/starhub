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
	cursor := int64(0)
	for {
		var users []model.User
		if err := model.DB.WithContext(ctx).Where("id > ?", cursor).Order("id").Limit(128).Find(&users).Error; err != nil {
			log.Print("daily reconciliation: user page unavailable")
			return
		}
		if len(users) == 0 {
			return
		}
		for _, user := range users {
			cursor = user.ID
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
}

func RunChangedDayReconciliationOnce(ctx context.Context) {
	rows, err := model.PendingReconciliationWork(ctx, time.Now())
	if err != nil {
		if ctx.Err() == nil {
			log.Print("pending reconciliation unavailable")
		}
		return
	}
	for _, row := range rows {
		if ctx.Err() != nil {
			return
		}
		record, changed, err := model.ReconcileBusinessDay(row.UserID, row.BusinessDaySnapshot)
		if err != nil {
			log.Print("changed business day reconciliation failed")
			continue
		}
		if changed && record.Alerted {
			NotifyReconciliationDiff(record.Diff, fmt.Sprintf("user %d business day %s: %s; nano difference %s", row.UserID, row.BusinessDate, record.State, record.DifferenceNano))
		}
		if err := model.AckReconciliationWork(ctx, row); err != nil {
			log.Print("reconciliation acknowledgment failed")
		}
	}
}

// Local 02:00 eligibility is evaluated against each frozen calendar, not UTC.
func RunDailyReconciliationLoop(ctx context.Context) {
	RunDailyReconciliationOnce(ctx)
	fullSweep := time.NewTicker(24 * time.Hour)
	defer fullSweep.Stop()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			RunChangedDayReconciliationOnce(ctx)
		case <-fullSweep.C:
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

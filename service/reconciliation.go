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

		collected, err := model.SumChargedLogMoney(user.ID, dayStart.Unix(), dayEnd.Unix())
		if err != nil {
			log.Printf("daily reconciliation: exact log sum for user %d failed: %v", user.ID, err)
			continue
		}
		account, verifyErr := model.AuditMoneyLedger(user.ID)
		current := common.QuotaMoney(user.UsedQuota, user.UsedQuotaRemainderNano)
		sequence := int64(0)
		if account != nil {
			sequence = account.Sequence
			if value, ok := new(big.Int).SetString(account.SpentNano, 10); ok {
				current = value
			}
		}
		previous := new(big.Int)
		if prev, err := model.ReconciliationBeforeDate(user.ID, dateStr); err == nil {
			previous = common.QuotaMoney(prev.UsedQuotaDelta, 0)
			if prev.AccountingVersion == 2 {
				if value, ok := new(big.Int).SetString(prev.UsedQuotaNanoSnapshot, 10); ok {
					previous = value
				}
			}
		}
		usedDelta := new(big.Int).Sub(current, previous)
		difference := new(big.Int).Sub(collected, usedDelta)
		difference.Abs(difference)
		alerted := false
		if difference.Cmp(common.QuotaMoney(reconciliationDiffThreshold, 0)) > 0 {
			NotifyReconciliationDiff(reconciliationQuotaProjection(difference), "user usage snapshot delta does not match daily log sum")
			alerted = true
		}
		if verifyErr != nil {
			NotifyReconciliationDiff(0, fmt.Sprintf("canonical money ledger verification failed for user %d", user.ID))
			alerted = true
		}
		failure := ""
		if verifyErr != nil {
			failure = "canonical_money_verification_failed"
		}
		record := &model.ReconciliationRecord{UserID: user.ID, Date: dateStr, LogQuotaSum: reconciliationQuotaProjection(collected), UsedQuotaDelta: reconciliationQuotaProjection(current), Diff: reconciliationQuotaProjection(difference), Alerted: alerted, AccountingVersion: 2, LogQuotaNanoSum: collected.String(), UsedQuotaNanoSnapshot: current.String(), DifferenceNano: difference.String(), LedgerVerified: verifyErr == nil, LedgerError: failure, MoneySequence: sequence}

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

func reconciliationQuotaProjection(amount *big.Int) int64 {
	whole, _, err := common.NanoQuotaParts(amount)
	if err != nil {
		return math.MaxInt64
	}
	return whole
}

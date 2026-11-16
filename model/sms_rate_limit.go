package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrSMSRequestLimited = errors.New("SMS request limit reached")

// SMSRequestLimitError carries the latest reset among all exhausted budgets.
type SMSRequestLimitError struct{ RetryAt int64 }

func (e *SMSRequestLimitError) Error() string { return ErrSMSRequestLimited.Error() }
func (e *SMSRequestLimitError) Unwrap() error { return ErrSMSRequestLimited }
func (e *SMSRequestLimitError) RetryAfterSeconds() int {
	return int(max(int64(1), e.RetryAt-time.Now().Unix()))
}

// SMSRequestBucket persists reservations before contacting a paid provider.
// Keys are digests; numbers, verification codes and credentials are not stored.
type SMSRequestBucket struct {
	Key       string `gorm:"primaryKey;size:64"`
	StartedAt int64
	Count     int
}

type SMSRequestLimit struct {
	Scope  string
	Window time.Duration
	Max    int
}

// ReserveSMSRequest checks all limits atomically, including concurrent callers.
// Failed provider requests retain their reservation to prevent retry abuse.
func ReserveSMSRequest(limits ...SMSRequestLimit) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var retryAt int64
		for _, limit := range limits {
			key := fmt.Sprintf("%x", sha256.Sum256([]byte(limit.Scope)))
			bucket := SMSRequestBucket{Key: key, StartedAt: time.Now().Unix()}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&bucket).Error; err != nil {
				return err
			}
			// Lock the bucket even when its count is exhausted. Otherwise a
			// concurrent window reset could produce an incorrect retry deadline.
			if err := tx.Model(&SMSRequestBucket{}).Where(map[string]any{"key": key}).UpdateColumn("count", gorm.Expr("count")).Error; err != nil {
				return err
			}
			now := time.Now().Unix()
			if err := tx.Model(&SMSRequestBucket{}).Where(map[string]any{"key": key}).Where("started_at <= ?", now-int64(limit.Window/time.Second)).Updates(map[string]any{"started_at": now, "count": 0}).Error; err != nil {
				return err
			}
			result := tx.Model(&SMSRequestBucket{}).Where(map[string]any{"key": key}).Where("count < ?", limit.Max).UpdateColumn("count", gorm.Expr("count + 1"))
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				var current SMSRequestBucket
				if err := tx.Where(map[string]any{"key": key}).First(&current).Error; err != nil {
					return err
				}
				retryAt = max(retryAt, current.StartedAt+int64(limit.Window/time.Second))
			}
		}
		if retryAt > 0 {
			// Returning an error rolls back every provisional increment and reset.
			return &SMSRequestLimitError{RetryAt: retryAt}
		}
		// Bound storage growth without retaining historical request identifiers.
		return tx.Where("started_at < ?", time.Now().Add(-24*time.Hour).Unix()).Delete(&SMSRequestBucket{}).Error
	})
}

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
	now := time.Now().Unix()
	return DB.Transaction(func(tx *gorm.DB) error {
		for _, limit := range limits {
			key := fmt.Sprintf("%x", sha256.Sum256([]byte(limit.Scope)))
			bucket := SMSRequestBucket{Key: key, StartedAt: now}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&bucket).Error; err != nil {
				return err
			}
			if err := tx.Model(&SMSRequestBucket{}).Where(map[string]any{"key": key}).Where("started_at <= ?", now-int64(limit.Window/time.Second)).Updates(map[string]any{"started_at": now, "count": 0}).Error; err != nil {
				return err
			}
			result := tx.Model(&SMSRequestBucket{}).Where(map[string]any{"key": key}).Where("count < ?", limit.Max).UpdateColumn("count", gorm.Expr("count + 1"))
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrSMSRequestLimited
			}
		}
		// Bound storage growth without retaining historical request identifiers.
		return tx.Where("started_at < ?", now-int64(24*time.Hour/time.Second)).Delete(&SMSRequestBucket{}).Error
	})
}

package model

import (
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
)

// QuotaSettlement freezes a completed synchronous request's bill before the
// money transaction. Payload contains accounting metadata only, never prompts.
type QuotaSettlement struct {
	ID             int64 `gorm:"primaryKey;autoIncrement"`
	UserID         int64 `gorm:"not null"`
	TokenID        int64 `gorm:"not null"`
	TokenUnlimited bool
	Delta          int64
	Actual         int64
	Payload        string `gorm:"type:text;not null"`
	CreatedAt      int64  `gorm:"autoCreateTime"`
	CompletedAt    int64  `gorm:"index;not null;default:0"`
}

var errQuotaAlreadySettled = errors.New("quota intent already settled")

func RecordQuotaSettlement(userID, tokenID int64, unlimited bool, delta, actual int64, entry *Log) (*QuotaSettlement, error) {
	if entry == nil || entry.ID != 0 || actual < 0 || delta > actual {
		return nil, errors.New("invalid quota settlement intent")
	}
	frozen := *entry
	frozen.UserID, frozen.TokenID = userID, tokenID
	frozen.Quota = actual
	if frozen.CreatedAt == 0 {
		frozen.CreatedAt = time.Now().Unix()
	}
	payload, err := json.Marshal(frozen)
	if err != nil {
		return nil, err
	}
	intent := &QuotaSettlement{UserID: userID, TokenID: tokenID, TokenUnlimited: unlimited, Delta: delta, Actual: actual, Payload: string(payload)}
	if err := DB.Create(intent).Error; err != nil {
		return nil, err
	}
	return intent, nil
}

func ListPendingQuotaSettlements(limit int) ([]QuotaSettlement, error) {
	var intents []QuotaSettlement
	err := DB.Where("completed_at = 0").Order("id ASC").Limit(limit).Find(&intents).Error
	return intents, err
}

// ApplyQuotaSettlement reads the persisted snapshot; stale workers cannot
// replace it or move funds twice. The claim rolls back with any failed log.
func ApplyQuotaSettlement(id int64) (*Log, bool, error) {
	var intent QuotaSettlement
	if err := DB.First(&intent, id).Error; err != nil {
		return nil, false, err
	}
	if intent.CompletedAt != 0 {
		return nil, false, nil
	}
	var entry Log
	if err := json.Unmarshal([]byte(intent.Payload), &entry); err != nil {
		return nil, false, err
	}
	err := settleQuota(intent.UserID, intent.TokenID, intent.TokenUnlimited, intent.Delta, intent.Actual, &entry, func(tx *gorm.DB) error {
		res := tx.Model(&QuotaSettlement{}).Where("id = ? AND completed_at = 0", id).UpdateColumn("completed_at", time.Now().Unix())
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errQuotaAlreadySettled
		}
		return nil
	})
	if errors.Is(err, errQuotaAlreadySettled) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &entry, true, nil
}

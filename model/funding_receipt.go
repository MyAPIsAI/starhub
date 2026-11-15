package model

import (
	"math/big"
	"time"

	"gorm.io/gorm"
)

// FundingReceipt records authorized business inputs independently of journal
// projections. It never imports simulated payment balances.
type FundingReceipt struct {
	BusinessDaySnapshot
	ID          int64  `gorm:"primaryKey"`
	UserID      int64  `gorm:"uniqueIndex:idx_funding_operation;index;not null"`
	OperationID string `gorm:"size:192;uniqueIndex:idx_funding_operation;not null"`
	Source      string `gorm:"size:32;not null"`
	AmountNano  string `gorm:"size:64;not null"`
	ActorID     int64
	CreatedAt   int64 `gorm:"autoCreateTime"`
}

func createFundingReceipt(tx *gorm.DB, userID int64, operation, source string, amount *big.Int, actorID int64) (*FundingReceipt, error) {
	day, err := captureBusinessDay(tx, time.Now())
	if err != nil {
		return nil, err
	}
	row := &FundingReceipt{BusinessDaySnapshot: day, UserID: userID, OperationID: operation, Source: source, AmountNano: amount.String(), ActorID: actorID}
	if err := tx.Create(row).Error; err != nil {
		return nil, err
	}
	if err := queueReconciliationDay(tx, userID, day); err != nil {
		return nil, err
	}
	return row, nil
}

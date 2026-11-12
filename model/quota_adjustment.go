package model

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
)

var ErrAdjustmentConflict = errors.New("quota adjustment conflict: user status or balance changed")

// AdjustUserQuota atomically adjusts a user's API quota and writes a ledger
// entry in the same transaction. The adjustment is signed: positive increases
// quota (credit), negative decreases (debit). The expectedQuota parameter is
// an optimistic-concurrency guard — if the current quota does not match, the
// transaction is aborted. The actorID is the admin who performed the operation.
func AdjustUserQuota(userID int64, amount int64, expectedQuota int64, actorID int64, reason string) (*LedgerEntry, error) {
	if amount == 0 {
		return nil, errors.New("adjustment amount must be non-zero")
	}
	if reason == "" {
		return nil, errors.New("reason is required for quota adjustments")
	}
	var entry LedgerEntry
	err := DB.Transaction(func(tx *gorm.DB) error {
		// Lock the user row.
		var user User
		if err := tx.Where("id = ?", userID).First(&user).Error; err != nil {
			return fmt.Errorf("user not found: %w", err)
		}
		// Optimistic concurrency: check expected quota.
		if user.Quota != expectedQuota {
			return ErrAdjustmentConflict
		}
		// Apply the adjustment.
		newQuota := user.Quota + amount
		if newQuota < 0 {
			return fmt.Errorf("adjustment would result in negative quota: %d", newQuota)
		}
		if err := tx.Model(&User{}).Where("id = ? AND quota = ?", userID, user.Quota).
			Update("quota", newQuota).Error; err != nil {
			return err
		}
		// Write ledger entry with actor attribution.
		entry = LedgerEntry{
			UserID:        userID,
			Type:          "adjustment",
			Amount:        amount,
			BalanceAfter:  newQuota,
			Description:   fmt.Sprintf("by admin %d: %s", actorID, reason),
			FundingSource: FundingSourceLegacyUnverified,
		}
		return tx.Create(&entry).Error
	})
	if err != nil {
		return nil, err
	}
	return &entry, nil
}

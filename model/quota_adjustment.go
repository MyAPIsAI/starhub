package model

import (
	"errors"
	"fmt"
	"math"
	"starhub/common"

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
		lock := tx.Model(&User{}).Where("id = ?", userID).UpdateColumn("quota", gorm.Expr("quota"))
		if lock.Error != nil {
			return lock.Error
		}
		if lock.RowsAffected != 1 {
			return ErrUserNotFound
		}
		var user User
		if err := tx.Where("id = ?", userID).First(&user).Error; err != nil {
			return fmt.Errorf("user not found: %w", err)
		}
		// Optimistic concurrency: check expected quota.
		if user.Quota != expectedQuota {
			return ErrAdjustmentConflict
		}
		account, err := ensureMoneyAccount(tx, user)
		if err != nil {
			return err
		}
		// Apply the adjustment.
		if user.Quota < 0 || (amount > 0 && user.Quota > math.MaxInt64-amount) || (amount < 0 && amount < -user.Quota) {
			return errors.New("quota adjustment exceeds balance bounds")
		}
		newQuota := user.Quota + amount
		if newQuota < 0 {
			return fmt.Errorf("adjustment would result in negative quota: %d", newQuota)
		}
		updated := tx.Model(&User{}).Where("id = ? AND quota = ?", userID, user.Quota).Update("quota", newQuota)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return ErrAdjustmentConflict
		}
		// Write ledger entry with actor attribution.
		entry = LedgerEntry{
			Account:                   LedgerAccountQuota,
			AmountUnit:                LedgerUnitLegacyQuota,
			UserID:                    userID,
			Type:                      "adjustment",
			Amount:                    amount,
			BalanceAfter:              newQuota,
			BalanceAfterRemainderNano: user.QuotaRemainderNano,
			Description:               fmt.Sprintf("by admin %d: %s", actorID, reason),
			FundingSource:             FundingSourceLegacyUnverified,
		}
		if err := tx.Create(&entry).Error; err != nil {
			return err
		}
		_, err = recordMoneyMovement(tx, account, moneyMovement{Available: common.QuotaMoney(amount, 0), Kind: "admin_adjustment", OperationID: fmt.Sprintf("legacy-adjustment:%d", entry.ID), ActorID: actorID, Reason: reason, FundingSource: "administrator"})
		return err
	})
	if err != nil {
		return nil, err
	}
	return &entry, nil
}

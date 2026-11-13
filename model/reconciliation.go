package model

import (
	"gorm.io/gorm"
	"math/big"
)

// ReconciliationRecord persists the result of one daily reconciliation pass
// for one user. It enables audit reconstruction and difference tracking over
// time. Each run at 02:00 UTC creates one row per active user.
type ReconciliationRecord struct {
	AccountingVersion     int    `gorm:"not null;default:0" json:"accounting_version"`
	LogQuotaNanoSum       string `gorm:"size:64;not null;default:'0'" json:"log_quota_nano_sum"`
	UsedQuotaNanoSnapshot string `gorm:"size:64;not null;default:'0'" json:"used_quota_nano_snapshot"`
	DifferenceNano        string `gorm:"size:64;not null;default:'0'" json:"difference_nano"`
	LedgerVerified        bool   `gorm:"not null;default:false" json:"ledger_verified"`
	LedgerError           string `gorm:"size:128;not null;default:''" json:"ledger_error"`
	MoneySequence         int64  `gorm:"not null;default:0" json:"money_sequence"`
	ID                    int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID                int64  `gorm:"index;not null" json:"user_id"`
	Date                  string `gorm:"size:10;index;not null" json:"date"` // YYYY-MM-DD UTC
	LogQuotaSum           int64  `gorm:"not null;default:0" json:"log_quota_sum"`
	UsedQuotaDelta        int64  `gorm:"not null;default:0" json:"used_quota_delta"`
	Diff                  int64  `gorm:"not null;default:0" json:"diff"` // abs(LogQuotaSum - UsedQuotaDelta)
	Alerted               bool   `gorm:"not null;default:false" json:"alerted"`
	CreatedAt             int64  `gorm:"autoCreateTime" json:"created_at"`
}

// InsertReconciliationRecord persists one reconciliation result. It is
// best-effort: a failure is logged but does not abort the run.
func InsertReconciliationRecord(r *ReconciliationRecord) error {
	return DB.Create(r).Error
}

// GetLatestReconciliationForUser returns the most recent reconciliation
// record for the given user, or nil if none exists.
func GetLatestReconciliationForUser(userID int64) (*ReconciliationRecord, error) {
	var r ReconciliationRecord
	err := DB.Where("user_id = ?", userID).Order("id DESC").First(&r).Error
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// AuditMoneyLedger returns the head and its verification from one read snapshot.
// Opening legacy quota does not import simulated wallet funds.
func AuditMoneyLedger(userID int64) (*MoneyAccount, error) {
	if _, err := ReadMoneyAccount(userID); err != nil {
		return nil, err
	}
	var account MoneyAccount
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&account, "user_id = ?", userID).Error; err != nil {
			return err
		}
		return verifyMoneyLedger(tx, userID)
	})
	return &account, err
}

func SumChargedLogMoney(userID, since, until int64) (*big.Int, error) {
	amounts, err := readGroupedLogMoney(DB.Model(&Log{}).Where("user_id = ? AND created_at >= ? AND created_at < ?", userID, since, until), "'all'")
	if err != nil {
		return nil, err
	}
	return new(big.Int).Set(&usageMoneyAt(amounts, "all").Charged), nil
}

func ReconciliationBeforeDate(userID int64, date string) (*ReconciliationRecord, error) {
	var record ReconciliationRecord
	err := DB.Where("user_id = ? AND date < ?", userID, date).Order("date DESC, id DESC").First(&record).Error
	return &record, err
}

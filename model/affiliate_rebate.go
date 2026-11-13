package model

import (
	"fmt"
	"math/big"
	"strconv"
	"time"

	"gorm.io/gorm"
	"starhub/common"
	"starhub/setting"
)

// AffiliateRebate is an outbox item created atomically with its source bill.
// Recipient, ratio and amount are frozen; later policy changes cannot redirect it.
type AffiliateRebate struct {
	QuotaRemainderNano int64 `gorm:"not null;default:0"`
	LogID              int64 `gorm:"primaryKey;autoIncrement:false"`
	UserID             int64 `gorm:"not null"`
	RecipientID        int64 `gorm:"not null"`
	Ratio              float64
	Quota              int64
	CreatedAt          int64 `gorm:"autoCreateTime"`
	CompletedAt        int64 `gorm:"index:idx_rebate_pending,priority:1;not null;default:0"`
	LastAttemptAt      int64 `gorm:"index:idx_rebate_pending,priority:2;not null;default:0"`
}

func queueAffiliateRebate(tx *gorm.DB, entry *Log) error {
	if common.QuotaMoney(entry.Quota, entry.QuotaRemainderNano).Sign() <= 0 {
		return nil
	}
	ratio := setting.AffRebateRatioValue()
	amount := common.QuotaFromFloat64(float64(entry.Quota) * ratio)
	remainder := int64(0)
	if entry.AmountUnit == common.MoneyUnitNano {
		factor, ok := new(big.Rat).SetString(strconv.FormatFloat(ratio, 'g', -1, 64))
		if !ok || factor.Sign() < 0 {
			return ErrMoneyConflict
		}
		fee := common.QuotaMoney(entry.Quota, entry.QuotaRemainderNano)
		value := new(big.Rat).Mul(new(big.Rat).SetInt(fee), factor)
		whole := new(big.Int).Quo(value.Num(), value.Denom())
		var err error
		amount, remainder, err = common.NanoQuotaParts(whole)
		if err != nil {
			return err
		}
	}
	if amount <= 0 && remainder <= 0 {
		return nil
	}
	var owner User
	if err := tx.Select("id", "inviter_id").First(&owner, entry.UserID).Error; err != nil {
		return err
	}
	if owner.InviterID <= 0 {
		return nil
	}
	return tx.Create(&AffiliateRebate{LogID: entry.ID, UserID: entry.UserID, RecipientID: owner.InviterID, Ratio: ratio, Quota: amount, QuotaRemainderNano: remainder}).Error
}

func ListPendingAffiliateRebates(limit int) ([]AffiliateRebate, error) {
	var items []AffiliateRebate
	err := DB.Where("completed_at = 0").Order("last_attempt_at ASC, log_id ASC").Limit(limit).Find(&items).Error
	return items, err
}

// DeliverAffiliateRebate claims the outbox item and credits the recipient in
// one transaction. A failed credit rolls back the claim and remains retryable.
func DeliverAffiliateRebate(logID int64) error {
	err := DB.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&AffiliateRebate{}).Where("log_id = ? AND completed_at = 0", logID).UpdateColumn("completed_at", time.Now().Unix())
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}
		var item AffiliateRebate
		if err := tx.First(&item, "log_id = ?", logID).Error; err != nil {
			return err
		}
		_, err := applyUserMoneyDelta(tx, item.RecipientID, common.QuotaMoney(item.Quota, item.QuotaRemainderNano), "affiliate_rebate", fmt.Sprintf("rebate:%d", item.LogID), "Collected usage affiliate rebate", "affiliate", 0)
		return err
	})
	if err != nil {
		// Move unavailable recipients behind untouched work instead of letting
		// a full page of failures starve later rebates forever.
		_ = DB.Model(&AffiliateRebate{}).Where("log_id = ? AND completed_at = 0", logID).UpdateColumn("last_attempt_at", time.Now().UnixNano()).Error
	}
	return err
}

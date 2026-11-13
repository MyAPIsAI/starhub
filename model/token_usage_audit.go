package model

import (
	"errors"
	"gorm.io/gorm"
	"starhub/common"
)

// Baselines make legacy counters explicit opening observations. Subsequent
// consumption must be independently reconstructible from immutable entries.
type TokenUsageBaseline struct {
	UserID          int64 `gorm:"primaryKey;autoIncrement:false"`
	TokenID         int64 `gorm:"primaryKey;autoIncrement:false"`
	PeriodID        int64 `gorm:"primaryKey;autoIncrement:false"`
	OpeningSequence int64
	OpeningNano     string `gorm:"size:64;not null"`
}

func attachTokenAccounting(tx *gorm.DB, account *MoneyAccount, entry *MoneyEntry) error {
	if entry.Kind != "consumption" || entry.LogID == 0 {
		return nil
	}
	var log Log
	if err := tx.First(&log, entry.LogID).Error; err != nil {
		return err
	}
	if log.UserID != account.UserID {
		return ErrMoneyConflict
	}
	entry.TokenID, entry.PeriodID = log.TokenID, log.QuotaPeriodID
	if entry.TokenID == 0 {
		return nil
	}
	paid, err := common.ParseNano(entry.SpentDeltaNano)
	if err != nil {
		return err
	}
	for _, periodID := range []int64{0, entry.PeriodID} {
		var baseline TokenUsageBaseline
		err := tx.First(&baseline, "user_id = ? AND token_id = ? AND period_id = ?", entry.UserID, entry.TokenID, periodID).Error
		if err == nil {
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var used = common.QuotaMoney(0, 0)
		if periodID == 0 {
			var token Token
			if err := tx.First(&token, "id = ? AND user_id = ?", entry.TokenID, entry.UserID).Error; err != nil {
				return err
			}
			used = common.QuotaMoney(token.UsedQuota, token.UsedQuotaRemainderNano)
		} else {
			var period TokenQuotaPeriod
			if err := tx.First(&period, "id = ? AND user_id = ? AND token_id = ?", periodID, entry.UserID, entry.TokenID).Error; err != nil {
				return err
			}
			used = common.QuotaMoney(period.UsedQuota, period.UsedQuotaRemainderNano)
		}
		used.Sub(used, paid)
		if used.Sign() < 0 {
			return ErrMoneyConflict
		}
		baseline = TokenUsageBaseline{UserID: entry.UserID, TokenID: entry.TokenID, PeriodID: periodID, OpeningSequence: account.Sequence, OpeningNano: used.String()}
		if err := tx.Create(&baseline).Error; err != nil {
			return err
		}
		if entry.PeriodID == 0 {
			break
		}
	}
	return nil
}

func verifyTokenAccounting(tx *gorm.DB, userID int64) error {
	var baselines []TokenUsageBaseline
	if err := tx.Where("user_id = ?", userID).Find(&baselines).Error; err != nil {
		return err
	}
	for _, b := range baselines {
		expected, err := common.ParseNano(b.OpeningNano)
		if err != nil {
			return err
		}
		q := tx.Where("user_id = ? AND token_id = ? AND sequence > ? AND kind = ?", userID, b.TokenID, b.OpeningSequence, "consumption")
		if b.PeriodID > 0 {
			q = q.Where("period_id = ?", b.PeriodID)
		}
		var entries []MoneyEntry
		if err := q.Find(&entries).Error; err != nil {
			return err
		}
		for _, e := range entries {
			paid, err := common.ParseNano(e.SpentDeltaNano)
			if err != nil {
				return err
			}
			expected.Add(expected, paid)
		}
		var actual = common.QuotaMoney(0, 0)
		if b.PeriodID == 0 {
			var token Token
			err := tx.First(&token, "id = ? AND user_id = ?", b.TokenID, userID).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			actual = common.QuotaMoney(token.UsedQuota, token.UsedQuotaRemainderNano)
		} else {
			var period TokenQuotaPeriod
			if err := tx.First(&period, "id = ? AND user_id = ?", b.PeriodID, userID).Error; err != nil {
				return err
			}
			actual = common.QuotaMoney(period.UsedQuota, period.UsedQuotaRemainderNano)
		}
		if actual.Cmp(expected) != 0 {
			return ErrMoneyConflict
		}
	}
	return nil
}

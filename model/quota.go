package model

import (
	"errors"

	"gorm.io/gorm"
)

var (
	ErrUserQuotaShort  = errors.New("user quota is not enough to process this request")
	ErrTokenQuotaShort = errors.New("token quota is not enough to process this request")
)

// The quota operations below are the money-moving core of the relay. They
// run inside a single write transaction each: SQLite serializes writers, so
// "check balance and take" executes as one atomic step — the row-lock
// (lockForUpdate) semantics of MySQL without the FOR UPDATE syntax, and the
// user / token pair can never end up half-reserved. Contending writers wait
// on the connection busy_timeout instead of erroring out.

// ReserveQuota atomically holds `amount` from the user and (when the token
// is not unlimited) from the token. It returns ErrUserQuotaShort /
// ErrTokenQuotaShort without touching any balance when the funds are
// insufficient.
func ReserveQuota(userID, tokenID int64, tokenUnlimited bool, amount int64) error {
	if amount <= 0 {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&User{}).
			Where("id = ? AND quota >= ?", userID, amount).
			Updates(map[string]any{"quota": gorm.Expr("quota - ?", amount)})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrUserQuotaShort
		}
		if !tokenUnlimited {
			res := tx.Model(&Token{}).
				Where("id = ? AND remain_quota >= ?", tokenID, amount).
				Updates(map[string]any{"remain_quota": gorm.Expr("remain_quota - ?", amount)})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return ErrTokenQuotaShort
			}
		}
		return nil
	})
}

// ReleaseQuota returns a reservation (full refund when a relay attempt
// fails). Token and user balances are restored together.
func ReleaseQuota(userID, tokenID int64, tokenUnlimited bool, amount int64) error {
	if amount <= 0 {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if !tokenUnlimited {
			if err := tx.Model(&Token{}).Where("id = ?", tokenID).
				Updates(map[string]any{"remain_quota": gorm.Expr("remain_quota + ?", amount)}).Error; err != nil {
				return err
			}
		}
		return tx.Model(&User{}).Where("id = ?", userID).
			Updates(map[string]any{"quota": gorm.Expr("quota + ?", amount)}).Error
	})
}

// SettleQuota balances a reservation against the actual cost in ONE
// transaction: the difference (actual - pre) is taken from or returned to
// the token and user (unconditionally — post-pay may go slightly negative),
// the used-quota counters advance by the actual cost, and the consumption
// log row is written. All-or-nothing, so the books always balance.
func SettleQuota(userID, tokenID int64, tokenUnlimited bool, delta, actual int64, entry *Log) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if delta > 0 {
			if !tokenUnlimited {
				if err := tx.Model(&Token{}).Where("id = ?", tokenID).
					Updates(map[string]any{"remain_quota": gorm.Expr("remain_quota - ?", delta)}).Error; err != nil {
					return err
				}
			}
			if err := tx.Model(&User{}).Where("id = ?", userID).
				Updates(map[string]any{"quota": gorm.Expr("quota - ?", delta)}).Error; err != nil {
				return err
			}
		} else if delta < 0 {
			if !tokenUnlimited {
				if err := tx.Model(&Token{}).Where("id = ?", tokenID).
					Updates(map[string]any{"remain_quota": gorm.Expr("remain_quota + ?", -delta)}).Error; err != nil {
					return err
				}
			}
			if err := tx.Model(&User{}).Where("id = ?", userID).
				Updates(map[string]any{"quota": gorm.Expr("quota + ?", -delta)}).Error; err != nil {
				return err
			}
		}
		if actual > 0 {
			if !tokenUnlimited {
				if err := tx.Model(&Token{}).Where("id = ?", tokenID).
					Updates(map[string]any{"used_quota": gorm.Expr("used_quota + ?", actual)}).Error; err != nil {
					return err
				}
			}
			if err := tx.Model(&User{}).Where("id = ?", userID).
				Updates(map[string]any{"used_quota": gorm.Expr("used_quota + ?", actual)}).Error; err != nil {
				return err
			}
			if entry != nil && entry.ChannelID > 0 {
				if err := tx.Model(&Channel{}).Where("id = ?", entry.ChannelID).
					Updates(map[string]any{"used_quota": gorm.Expr("used_quota + ?", actual)}).Error; err != nil {
					return err
				}
			}
		}
		if entry != nil {
			return tx.Create(entry).Error
		}
		return nil
	})
}

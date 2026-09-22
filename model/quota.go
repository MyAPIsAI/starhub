package model

import (
	"errors"
	"log"

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

// SettleQuota settles both balances, usage counters and the log atomically.
// A shortfall caps the extra charge at the smaller available balance. Both
// balances and all counters use the same collected amount; the log retains
// the uncollected cost separately. The caller's log is updated only after
// commit so downstream rebates can use the amount actually collected.
func SettleQuota(userID, tokenID int64, tokenUnlimited bool, delta, actual int64, entry *Log) error {
	if actual < 0 || delta > actual {
		return errors.New("invalid quota settlement amounts")
	}
	var settled Log
	if entry != nil {
		settled = *entry
	}
	settled.UserID, settled.TokenID = userID, tokenID
	settled.Quota, settled.UnfundedQuota = actual, 0
	err := DB.Transaction(func(tx *gorm.DB) error {
		// Acquire SQLite's write lock before reading balances. A deferred
		// read-then-write transaction can otherwise fail with SQLITE_BUSY.
		res := tx.Model(&User{}).Where("id = ?", userID).UpdateColumn("quota", gorm.Expr("quota"))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrUserNotFound
		}
		var token Token
		if err := tx.Where("id = ? AND user_id = ?", tokenID, userID).First(&token).Error; err != nil {
			return err
		}
		if delta > 0 {
			var user User
			if err := tx.First(&user, userID).Error; err != nil {
				return err
			}
			collectedDelta := min(delta, max(user.Quota, 0))
			if !tokenUnlimited {
				collectedDelta = min(collectedDelta, max(token.RemainQuota, 0))
			}
			settled.UnfundedQuota = delta - collectedDelta
			settled.Quota = actual - settled.UnfundedQuota
			if !tokenUnlimited {
				if err := tx.Model(&Token{}).Where("id = ?", tokenID).
					Updates(map[string]any{"remain_quota": gorm.Expr("remain_quota - ?", collectedDelta)}).Error; err != nil {
					return err
				}
			}
			if err := tx.Model(&User{}).Where("id = ?", userID).
				Updates(map[string]any{"quota": gorm.Expr("quota - ?", collectedDelta)}).Error; err != nil {
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
		if settled.Quota > 0 {
			if !tokenUnlimited {
				if err := tx.Model(&Token{}).Where("id = ?", tokenID).
					Updates(map[string]any{"used_quota": gorm.Expr("used_quota + ?", settled.Quota)}).Error; err != nil {
					return err
				}
			}
			if err := tx.Model(&User{}).Where("id = ?", userID).
				Updates(map[string]any{"used_quota": gorm.Expr("used_quota + ?", settled.Quota)}).Error; err != nil {
				return err
			}
			if entry != nil && entry.ChannelID > 0 {
				if err := tx.Model(&Channel{}).Where("id = ?", entry.ChannelID).
					Updates(map[string]any{"used_quota": gorm.Expr("used_quota + ?", settled.Quota)}).Error; err != nil {
					return err
				}
			}
		}
		if entry != nil || settled.UnfundedQuota > 0 {
			return tx.Create(&settled).Error
		}
		return nil
	})
	if err != nil {
		return err
	}
	if entry != nil {
		*entry = settled
	}
	if settled.UnfundedQuota > 0 {
		log.Printf("[quota_shortfall] user_id=%d token_id=%d log_id=%d actual=%d charged=%d unfunded=%d",
			userID, tokenID, settled.ID, actual, settled.Quota, settled.UnfundedQuota)
	}
	return nil
}

package model

import (
	"errors"
	"fmt"
	"log"
	"math"
	"math/big"
	"starhub/common"
	"time"

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
func ReserveQuota(userID, tokenID int64, tokenUnlimited bool, amount int64, admissionID ...string) error {
	if amount <= 0 {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		return reserveQuota(tx, userID, tokenID, tokenUnlimited, amount, admissionID...)
	})
}

func reserveQuota(tx *gorm.DB, userID, tokenID int64, tokenUnlimited bool, amount int64, admissionID ...string) error {
	if nanoAdmission(quotaAdmissionID(admissionID)) {
		return reserveNanoQuota(tx, userID, tokenID, tokenUnlimited, amount, admissionID)
	}
	_, account, err := lockMoneyUser(tx, userID)
	if err != nil {
		return err
	}
	res := tx.Model(&User{}).
		Where("id = ? AND quota >= ?", userID, amount).
		Updates(map[string]any{"quota": gorm.Expr("quota - ?", amount)})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrUserQuotaShort
	}
	if id := quotaAdmissionID(admissionID); id != "" {
		if err := reserveTokenAdmission(tx, userID, tokenID, tokenUnlimited, amount, id, time.Now()); err != nil {
			return err
		}
		var reservation QuotaReservation
		if err := tx.First(&reservation, "id = ?", id).Error; err != nil {
			return err
		}
		if reservation.PeriodID > 0 {
			return recordLegacyHold(tx, account, userID, tokenID, amount, id)
		}
	} else {
		var token Token
		if err := tx.Select("id", "quota_period").Where("id = ?", tokenID).Find(&token).Error; err != nil {
			return err
		}
		if tokenPeriodic(&token) {
			return ErrQuotaReservation
		}
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
	return recordLegacyHold(tx, account, userID, tokenID, amount, quotaAdmissionID(admissionID))
}

// ReleaseQuota returns a reservation (full refund when a relay attempt
// fails). Token and user balances are restored together.
func ReleaseQuota(userID, tokenID int64, tokenUnlimited bool, amount int64, admissionID ...string) error {
	if amount <= 0 {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if nanoAdmission(quotaAdmissionID(admissionID)) {
			return releaseNanoQuota(tx, userID, tokenID, amount, quotaAdmissionID(admissionID))
		}
		_, account, err := lockMoneyUser(tx, userID)
		if err != nil {
			return err
		}
		if id := quotaAdmissionID(admissionID); id != "" {
			var before QuotaReservation
			if err := tx.First(&before, "id = ?", id).Error; err != nil {
				return err
			}
			if err := releaseTokenAdmission(tx, userID, tokenID, amount, id, time.Now()); err != nil {
				return err
			}
			if before.CompletedAt != 0 {
				return nil
			}
			_, err := recordMoneyMovement(tx, account, moneyMovement{Available: common.QuotaMoney(amount, 0), Reserved: new(big.Int).Neg(common.QuotaMoney(amount, 0)), Kind: "release", OperationID: id + ":release", ReservationID: id, Reason: "Release legacy quota reservation", FundingSource: "spendable_quota"})
			return err
		}
		var token Token
		if err := tx.Select("id", "quota_period").Where("id = ?", tokenID).Find(&token).Error; err != nil {
			return err
		}
		if tokenPeriodic(&token) {
			return ErrQuotaReservation
		}
		if !tokenUnlimited {
			if err := tx.Model(&Token{}).Where("id = ?", tokenID).
				Updates(map[string]any{"remain_quota": gorm.Expr("remain_quota + ?", amount)}).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&User{}).Where("id = ?", userID).Updates(map[string]any{"quota": gorm.Expr("quota + ?", amount)}).Error; err != nil {
			return err
		}
		_, err = recordMoneyMovement(tx, account, moneyMovement{Available: common.QuotaMoney(amount, 0), Reserved: new(big.Int).Neg(common.QuotaMoney(amount, 0)), Kind: "release", OperationID: legacyMoneyOperation("legacy_release", "", amount), Reason: "Release legacy untracked reservation", FundingSource: "spendable_quota"})
		return err
	})
}

// SettleQuota settles both balances, usage counters and the log atomically.
// A shortfall caps the extra charge at the smaller available balance. Both
// balances and all counters use the same collected amount; the log retains
// the uncollected cost separately. The caller's log is updated only after
// commit so downstream rebates can use the amount actually collected.
func SettleQuota(userID, tokenID int64, tokenUnlimited bool, delta, actual int64, entry *Log) error {
	return settleQuota(userID, tokenID, tokenUnlimited, delta, actual, entry, nil)
}

func settleQuota(userID, tokenID int64, tokenUnlimited bool, delta, actual int64, entry *Log, claim func(*gorm.DB) error) error {
	if entry != nil && (entry.AmountUnit == common.MoneyUnitNano || nanoAdmission(entry.QuotaReservationID)) {
		return settleNanoQuota(userID, tokenID, delta, actual, entry, claim)
	}
	if actual < 0 || delta > actual || delta < actual-math.MaxInt64 {
		return errors.New("invalid quota settlement amounts")
	}
	var settled Log
	if entry != nil {
		settled = *entry
	}
	settled.UserID, settled.TokenID = userID, tokenID
	settled.Quota, settled.UnfundedQuota = actual, 0
	err := DB.Transaction(func(tx *gorm.DB) error {
		_, account, err := lockMoneyUser(tx, userID)
		if err != nil {
			return err
		}
		// Acquire SQLite's write lock before reading balances. A deferred
		// read-then-write transaction can otherwise fail with SQLITE_BUSY.
		res := tx.Model(&User{}).Where("id = ?", userID).UpdateColumn("quota", gorm.Expr("quota"))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrUserNotFound
		}
		if claim != nil {
			if err := claim(tx); err != nil {
				return err
			}
		}
		var token Token
		if err := tx.Where("id = ? AND user_id = ?", tokenID, userID).First(&token).Error; err != nil {
			if claim == nil || !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			// An accepted task outlives deletion of its key. Refund the owner,
			// but do not collect extra limited-key quota from an absent key.
			var exists int64
			if queryErr := tx.Model(&Token{}).Where("id = ?", tokenID).Count(&exists).Error; queryErr != nil {
				return queryErr
			}
			if exists != 0 {
				return err
			}
		}
		var reservation *QuotaReservation
		if settled.QuotaReservationID != "" {
			var r QuotaReservation
			if err := tx.First(&r, "id = ?", settled.QuotaReservationID).Error; err != nil {
				return err
			}
			if r.UserID != userID || r.TokenID != tokenID || r.CompletedAt != 0 || r.Amount != actual-delta {
				return ErrQuotaReservation
			}
			reservation = &r
			settled.AdmittedAt = r.AdmittedAt
			assignReservationDay(&settled, r)
			at := time.Unix(r.AdmittedAt, 0).UTC()
			settled.AdmissionDay = time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC).Unix()
			settled.QuotaPeriodID = r.PeriodID
		} else if tokenPeriodic(&token) {
			return ErrQuotaReservation
		}
		if reservation != nil && reservation.PeriodID > 0 {
			if err := settleTokenAdmission(tx, &token, *reservation, delta, actual, &settled, time.Now()); err != nil {
				return err
			}
		} else if delta > 0 {
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
		if reservation != nil {
			if err := tx.Model(&QuotaReservation{}).Where("id = ? AND completed_at = 0", reservation.ID).Update("completed_at", time.Now().Unix()).Error; err != nil {
				return err
			}
		}
		if settled.Quota > 0 {
			// used_quota is a usage counter, not a balance. It must be
			// updated for unlimited tokens too so the key-level consumption
			// statistics remain accurate.
			if err := tx.Model(&Token{}).Where("id = ?", tokenID).
				Updates(map[string]any{"used_quota": gorm.Expr("used_quota + ?", settled.Quota)}).Error; err != nil {
				return err
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
			if settled.BusinessDate == "" {
				settled.BusinessDaySnapshot = legacyBusinessDay(settled.AdmittedAt)
			}
			if err := tx.Create(&settled).Error; err != nil {
				return err
			}
			if err := queueAffiliateRebate(tx, &settled); err != nil {
				return err
			}
		}
		pre := actual - delta
		operation := legacyMoneyOperation("legacy_settle", settled.QuotaReservationID, settled.ID)
		if settled.QuotaReservationID != "" {
			operation = settled.QuotaReservationID + ":settle"
		} else if settled.ID > 0 {
			operation = fmt.Sprintf("legacy-log:%d", settled.ID)
		}
		_, err = recordMoneyMovement(tx, account, moneyMovement{Available: common.QuotaMoney(pre-settled.Quota, 0), Reserved: new(big.Int).Neg(common.QuotaMoney(pre, 0)), Spent: common.QuotaMoney(settled.Quota, 0), Kind: "consumption", OperationID: operation, ReservationID: settled.QuotaReservationID, LogID: settled.ID, Reason: "Settle legacy captured usage", FundingSource: "spendable_quota"})
		return err
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

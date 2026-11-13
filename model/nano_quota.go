package model

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"starhub/common"
)

func periodMoneyAvailable(p TokenQuotaPeriod) *big.Int {
	value := common.QuotaMoney(p.QuotaLimit, p.QuotaLimitRemainderNano)
	for _, amount := range []*big.Int{common.QuotaMoney(p.UsedQuota, p.UsedQuotaRemainderNano), common.QuotaMoney(p.CarryQuota, p.CarryQuotaRemainderNano), common.QuotaMoney(p.ReservedQuota, p.ReservedQuotaRemainderNano)} {
		value.Sub(value, amount)
	}
	if value.Sign() < 0 {
		return new(big.Int)
	}
	return value
}

func reserveNanoQuota(tx *gorm.DB, userID, tokenID int64, unlimited bool, amount int64, args []string) error {
	if amount < 0 || amount > common.MaxNanoPerRequest {
		return ErrQuotaReservation
	}
	id := quotaAdmissionID(args)
	if !nanoAdmission(id) || len(id) > 64 {
		return ErrQuotaReservation
	}
	user, account, err := lockMoneyUser(tx, userID)
	if err != nil {
		return err
	}
	var token Token
	if err := tx.First(&token, "id = ? AND user_id = ?", tokenID, userID).Error; err != nil {
		return err
	}
	if err := refreshTokenPeriod(tx, &token, time.Now()); err != nil {
		return err
	}
	var reservation QuotaReservation
	err = tx.First(&reservation, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		reservation = QuotaReservation{ID: id, UserID: userID, TokenID: tokenID, PeriodID: token.QuotaPeriodID, Limited: !unlimited || tokenPeriodic(&token), AdmittedAt: time.Now().Unix()}
		reservation.BusinessDaySnapshot, err = captureBusinessDay(tx, time.Unix(reservation.AdmittedAt, 0))
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if reservation.UserID != userID || reservation.TokenID != tokenID || reservation.CompletedAt != 0 {
		return ErrQuotaReservation
	}
	target := amount
	if len(args) > 1 {
		if !strings.HasPrefix(args[1], "target:") {
			return ErrQuotaReservation
		}
		target, err = strconv.ParseInt(strings.TrimPrefix(args[1], "target:"), 10, 64)
		if err != nil || target < amount || target > common.MaxNanoPerRequest {
			return ErrQuotaReservation
		}
	}
	if target <= reservation.Amount {
		return nil
	}
	delta := target - reservation.Amount
	change := big.NewInt(delta)
	available := common.QuotaMoney(user.Quota, user.QuotaRemainderNano)
	if available.Cmp(change) < 0 {
		return ErrUserQuotaShort
	}
	if reservation.PeriodID > 0 {
		var period TokenQuotaPeriod
		if err := tx.First(&period, reservation.PeriodID).Error; err != nil {
			return err
		}
		if periodMoneyAvailable(period).Cmp(change) < 0 {
			return ErrTokenQuotaShort
		}
		held := common.QuotaMoney(period.ReservedQuota, period.ReservedQuotaRemainderNano)
		held.Add(held, change)
		if err := setMoneyPair(tx, &TokenQuotaPeriod{}, period.ID, "reserved_quota", held); err != nil {
			return err
		}
		if token.QuotaPeriodID == period.ID {
			if err := tx.First(&period, period.ID).Error; err != nil {
				return err
			}
			if err := syncTokenPeriod(tx, &token, period); err != nil {
				return err
			}
		}
	} else if reservation.Limited {
		remaining := common.QuotaMoney(token.RemainQuota, token.RemainQuotaRemainderNano)
		if remaining.Cmp(change) < 0 {
			return ErrTokenQuotaShort
		}
		remaining.Sub(remaining, change)
		if err := setMoneyPair(tx, &Token{}, token.ID, "remain_quota", remaining); err != nil {
			return err
		}
	}
	available.Sub(available, change)
	if err := setMoneyPair(tx, &User{}, userID, "quota", available); err != nil {
		return err
	}
	reservation.Amount = target
	if err := tx.Save(&reservation).Error; err != nil {
		return err
	}
	_, err = recordMoneyMovement(tx, account, moneyMovement{Available: big.NewInt(-delta), Reserved: change, Kind: "reservation", OperationID: fmt.Sprintf("%s:hold:%d", id, target), ReservationID: id, Reason: "Reserve estimated upstream charge", FundingSource: "spendable_quota"})
	return err
}

func releaseNanoQuota(tx *gorm.DB, userID, tokenID, amount int64, id string) error {
	user, account, err := lockMoneyUser(tx, userID)
	if err != nil {
		return err
	}
	var reservation QuotaReservation
	if err := tx.First(&reservation, "id = ?", id).Error; err != nil {
		return err
	}
	if reservation.UserID != userID || reservation.TokenID != tokenID || reservation.Amount != amount {
		return ErrQuotaReservation
	}
	if reservation.CompletedAt != 0 {
		if reservation.Released {
			return nil
		}
		return ErrQuotaReservation
	}
	var token Token
	err = tx.First(&token, "id = ? AND user_id = ?", tokenID, userID).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	refund := big.NewInt(amount)
	if reservation.PeriodID > 0 {
		var period TokenQuotaPeriod
		if err := tx.First(&period, reservation.PeriodID).Error; err != nil {
			return err
		}
		held := common.QuotaMoney(period.ReservedQuota, period.ReservedQuotaRemainderNano)
		held.Sub(held, refund)
		if held.Sign() < 0 {
			return ErrQuotaReservation
		}
		if err := setMoneyPair(tx, &TokenQuotaPeriod{}, period.ID, "reserved_quota", held); err != nil {
			return err
		}
		if token.QuotaPeriodID == period.ID {
			if err := tx.First(&period, period.ID).Error; err != nil {
				return err
			}
			if err := syncTokenPeriod(tx, &token, period); err != nil {
				return err
			}
		}
	} else if reservation.Limited && token.ID != 0 {
		remaining := common.QuotaMoney(token.RemainQuota, token.RemainQuotaRemainderNano)
		remaining.Add(remaining, refund)
		if err := setMoneyPair(tx, &Token{}, token.ID, "remain_quota", remaining); err != nil {
			return err
		}
	}
	available := common.QuotaMoney(user.Quota, user.QuotaRemainderNano)
	available.Add(available, refund)
	if err := setMoneyPair(tx, &User{}, user.ID, "quota", available); err != nil {
		return err
	}
	reservation.CompletedAt, reservation.Released = time.Now().Unix(), true
	if err := tx.Save(&reservation).Error; err != nil {
		return err
	}
	_, err = recordMoneyMovement(tx, account, moneyMovement{Available: refund, Reserved: big.NewInt(-amount), Kind: "release", OperationID: id + ":release", ReservationID: id, Reason: "Release unused upstream reservation", FundingSource: "spendable_quota"})
	return err
}

func settleNanoQuota(userID, tokenID int64, delta, actual int64, entry *Log, claim func(*gorm.DB) error) error {
	if entry == nil || actual < 0 || actual > common.MaxNanoPerRequest || delta > actual || delta < actual-math.MaxInt64 {
		return ErrQuotaReservation
	}
	pre := actual - delta
	settled := *entry
	err := DB.Transaction(func(tx *gorm.DB) error {
		user, account, err := lockMoneyUser(tx, userID)
		if err != nil {
			return err
		}
		if claim != nil {
			if err := claim(tx); err != nil {
				return err
			}
		}
		var token Token
		err = tx.First(&token, "id = ? AND user_id = ?", tokenID, userID).Error
		if err != nil && (!errors.Is(err, gorm.ErrRecordNotFound) || claim == nil) {
			return err
		}
		if token.ID != 0 {
			if err := refreshTokenPeriod(tx, &token, time.Now()); err != nil {
				return err
			}
		}
		var reservation QuotaReservation
		if err := tx.First(&reservation, "id = ?", entry.QuotaReservationID).Error; err != nil {
			return err
		}
		if !nanoAdmission(reservation.ID) || reservation.UserID != userID || reservation.TokenID != tokenID || reservation.CompletedAt != 0 || reservation.Amount != pre {
			return ErrQuotaReservation
		}
		available := common.QuotaMoney(user.Quota, user.QuotaRemainderNano)
		var period TokenQuotaPeriod
		if reservation.PeriodID > 0 {
			if err := tx.First(&period, reservation.PeriodID).Error; err != nil {
				return err
			}
		}
		collectedDelta := delta
		if delta > 0 {
			cap := big.NewInt(delta)
			if available.Cmp(cap) < 0 {
				cap.Set(available)
			}
			if reservation.PeriodID > 0 {
				if left := periodMoneyAvailable(period); left.Cmp(cap) < 0 {
					cap.Set(left)
				}
			} else if reservation.Limited {
				left := common.QuotaMoney(token.RemainQuota, token.RemainQuotaRemainderNano)
				if token.ID == 0 {
					left.SetInt64(0)
				}
				if left.Cmp(cap) < 0 {
					cap.Set(left)
				}
			}
			collectedDelta = cap.Int64()
		}
		paid := pre + collectedDelta
		shortfall := actual - paid
		available.Sub(available, big.NewInt(collectedDelta))
		if err := setMoneyPair(tx, &User{}, userID, "quota", available); err != nil {
			return err
		}
		used := common.QuotaMoney(user.UsedQuota, user.UsedQuotaRemainderNano)
		used.Add(used, big.NewInt(paid))
		if err := setMoneyPair(tx, &User{}, userID, "used_quota", used); err != nil {
			return err
		}
		if reservation.PeriodID > 0 {
			held := common.QuotaMoney(period.ReservedQuota, period.ReservedQuotaRemainderNano)
			held.Sub(held, big.NewInt(pre))
			if held.Sign() < 0 {
				return ErrQuotaReservation
			}
			periodUsed := common.QuotaMoney(period.UsedQuota, period.UsedQuotaRemainderNano)
			periodUsed.Add(periodUsed, big.NewInt(paid))
			if err := setMoneyPair(tx, &TokenQuotaPeriod{}, period.ID, "reserved_quota", held); err != nil {
				return err
			}
			if err := setMoneyPair(tx, &TokenQuotaPeriod{}, period.ID, "used_quota", periodUsed); err != nil {
				return err
			}
			if token.QuotaPeriodID == period.ID {
				if err := tx.First(&period, period.ID).Error; err != nil {
					return err
				}
				if err := syncTokenPeriod(tx, &token, period); err != nil {
					return err
				}
			}
		} else if reservation.Limited && token.ID != 0 {
			remaining := common.QuotaMoney(token.RemainQuota, token.RemainQuotaRemainderNano)
			remaining.Sub(remaining, big.NewInt(collectedDelta))
			if err := setMoneyPair(tx, &Token{}, token.ID, "remain_quota", remaining); err != nil {
				return err
			}
		}
		if token.ID != 0 {
			tokenUsed := common.QuotaMoney(token.UsedQuota, token.UsedQuotaRemainderNano)
			tokenUsed.Add(tokenUsed, big.NewInt(paid))
			if err := setMoneyPair(tx, &Token{}, token.ID, "used_quota", tokenUsed); err != nil {
				return err
			}
		}
		if settled.ChannelID > 0 {
			var channel Channel
			err := tx.First(&channel, settled.ChannelID).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err == nil {
				channelUsed := common.QuotaMoney(channel.UsedQuota, channel.UsedQuotaRemainderNano)
				channelUsed.Add(channelUsed, big.NewInt(paid))
				if err := setMoneyPair(tx, &Channel{}, channel.ID, "used_quota", channelUsed); err != nil {
					return err
				}
			}
		}
		settled.UserID, settled.TokenID, settled.AmountUnit = userID, tokenID, common.MoneyUnitNano
		settled.Quota, settled.QuotaRemainderNano, err = common.NanoQuotaParts(big.NewInt(paid))
		if err != nil {
			return err
		}
		settled.UnfundedQuota, settled.UnfundedQuotaRemainderNano, err = common.NanoQuotaParts(big.NewInt(shortfall))
		if err != nil {
			return err
		}
		if settled.ExactFeeNanoUSD == "" {
			settled.ExactFeeNanoUSD = strconv.FormatInt(actual, 10)
		}
		settled.AdmittedAt, settled.QuotaPeriodID = reservation.AdmittedAt, reservation.PeriodID
		assignReservationDay(&settled, reservation)
		at := time.Unix(reservation.AdmittedAt, 0).UTC()
		settled.AdmissionDay = time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC).Unix()
		if err := tx.Create(&settled).Error; err != nil {
			return err
		}
		reservation.CompletedAt = time.Now().Unix()
		if err := tx.Save(&reservation).Error; err != nil {
			return err
		}
		if _, err := recordMoneyMovement(tx, account, moneyMovement{Available: big.NewInt(-collectedDelta), Reserved: big.NewInt(-pre), Spent: big.NewInt(paid), Kind: "consumption", OperationID: reservation.ID + ":settle", ReservationID: reservation.ID, LogID: settled.ID, Reason: "Settle captured upstream usage", FundingSource: "spendable_quota"}); err != nil {
			return err
		}
		return queueAffiliateRebate(tx, &settled)
	})
	if err == nil {
		*entry = settled
	}
	return err
}

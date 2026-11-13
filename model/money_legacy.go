package model

import (
	"fmt"
	"math/big"

	"gorm.io/gorm"
	"starhub/common"
)

func legacyMoneyOperation(kind, id string, amount int64) string {
	if id == "" {
		return kind + ":" + common.GenerateSessionKey()
	}
	return fmt.Sprintf("%s:%s:%d", id, kind, amount)
}

func recordLegacyHold(tx *gorm.DB, account *MoneyAccount, userID, tokenID, amount int64, id string) error {
	target := amount
	if id != "" {
		var reservation QuotaReservation
		if err := tx.First(&reservation, "id = ?", id).Error; err != nil {
			return err
		}
		target = reservation.Amount
	}
	value := common.QuotaMoney(amount, 0)
	_, err := recordMoneyMovement(tx, account, moneyMovement{Available: new(big.Int).Neg(value), Reserved: value, Kind: "reservation", OperationID: legacyMoneyOperation("legacy_hold", id, target), ReservationID: id, Reason: "Reserve legacy quota", FundingSource: "spendable_quota"})
	return err
}

func applyUserMoneyDelta(tx *gorm.DB, userID int64, amount *big.Int, kind, operation, reason, source string, actorID int64) (*MoneyEntry, error) {
	u, account, err := lockMoneyUser(tx, userID)
	if err != nil {
		return nil, err
	}
	available := common.QuotaMoney(u.Quota, u.QuotaRemainderNano)
	available.Add(available, amount)
	if available.Sign() < 0 {
		return nil, ErrUserQuotaShort
	}
	if err := setMoneyPair(tx, &User{}, userID, "quota", available); err != nil {
		return nil, err
	}
	return recordMoneyMovement(tx, account, moneyMovement{Available: amount, Kind: kind, OperationID: operation, Reason: reason, FundingSource: source, ActorID: actorID})
}

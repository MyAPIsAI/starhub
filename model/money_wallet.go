package model

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"

	"gorm.io/gorm"
	"starhub/common"
)

var ErrMoneyAuthorization = errors.New("money adjustment requires an active root account")
var ErrMoneyInput = errors.New("invalid money adjustment")
var moneyOperationID = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,64}$`)

type MoneyAdjustment struct {
	ActorID, UserID                                              int64
	OperationID, AmountNano, ExpectedNano, Reason, RequestID, IP string
}

func ReadMoneyAccount(userID int64) (*MoneyAccount, error) {
	var result *MoneyAccount
	err := DB.Transaction(func(tx *gorm.DB) error {
		_, account, err := lockMoneyUser(tx, userID)
		if err != nil {
			return err
		}
		result = account
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// AdjustMoneyBalance accepts only audited server-authorized grants or debits.
// Simulated recharge state and caller-provided funding claims are never inputs.
func AdjustMoneyBalance(input MoneyAdjustment) (*MoneyEntry, error) {
	input.Reason = strings.TrimSpace(input.Reason)
	amount, err := common.ParseNano(input.AmountNano)
	if err != nil || amount.Sign() == 0 || !moneyOperationID.MatchString(input.OperationID) || input.UserID <= 0 || input.ActorID <= 0 || input.Reason == "" || len(input.Reason) > 256 {
		return nil, ErrMoneyInput
	}
	expected, err := common.ParseNano(input.ExpectedNano)
	if err != nil || expected.Sign() < 0 {
		return nil, ErrMoneyInput
	}
	if _, _, err := common.NanoQuotaParts(amount); err != nil {
		return nil, ErrMoneyInput
	}
	raw, _ := json.Marshal(struct {
		Actor, User                         int64
		Operation, Amount, Expected, Reason string
	}{input.ActorID, input.UserID, input.OperationID, amount.String(), expected.String(), input.Reason})
	digest := fmt.Sprintf("%x", sha256.Sum256(raw))
	operation := "admin:" + input.OperationID
	var result *MoneyEntry
	err = DB.Transaction(func(tx *gorm.DB) error {
		user, account, err := lockMoneyUser(tx, input.UserID)
		if err != nil {
			return err
		}
		var actor User
		if err := tx.First(&actor, "id = ? AND role = ? AND status = ?", input.ActorID, common.RoleRoot, common.StatusEnabled).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrMoneyAuthorization
			}
			return err
		}
		var prior MoneyEntry
		if err := tx.First(&prior, "user_id = ? AND operation_id = ?", input.UserID, operation).Error; err == nil {
			if prior.RequestDigest != digest {
				return ErrMoneyConflict
			}
			result = &prior
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if account.AvailableNano != expected.String() {
			return ErrMoneyConflict
		}
		after := common.QuotaMoney(user.Quota, user.QuotaRemainderNano)
		after.Add(after, amount)
		if after.Sign() < 0 {
			return ErrMoneyInput
		}
		if err := setMoneyPair(tx, &User{}, user.ID, "quota", after); err != nil {
			return err
		}
		kind := "admin_credit"
		if amount.Sign() < 0 {
			kind = "admin_debit"
		}
		result, err = recordMoneyMovement(tx, account, moneyMovement{Available: amount, Kind: kind, OperationID: operation, Digest: digest, Reason: input.Reason, FundingSource: "administrator_grant", ActorID: input.ActorID})
		if err != nil {
			return err
		}
		return tx.Create(&AuditLog{UserID: actor.ID, Username: actor.Username, Method: "POST", Path: "/api/wallet/adjustments", Status: 200, RequestID: input.RequestID, IP: input.IP}).Error
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func ListMoneyEntries(userID int64, page, size int) ([]MoneyEntry, int64, error) {
	if page < 1 || page > 1000000 || size < 1 || size > 1000 {
		return nil, 0, ErrMoneyInput
	}
	rows := []MoneyEntry{}
	var total int64
	err := DB.Transaction(func(tx *gorm.DB) error {
		q := tx.Model(&MoneyEntry{}).Where("user_id = ?", userID)
		if err := q.Count(&total).Error; err != nil {
			return err
		}
		return q.Order("sequence DESC").Offset((page - 1) * size).Limit(size).Find(&rows).Error
	})
	return rows, total, err
}

func MoneyTotalAvailableAndHeld(account MoneyAccount) string {
	available, _ := new(big.Int).SetString(account.AvailableNano, 10)
	held, _ := new(big.Int).SetString(account.ReservedNano, 10)
	return new(big.Int).Add(available, held).String()
}

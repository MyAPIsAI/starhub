package model

import (
	"context"
	"errors"
	"math/big"
	"strings"

	"gorm.io/gorm"
	"starhub/common"
)

func APIBalanceNano(user *User) *big.Int {
	return new(big.Int).Add(new(big.Int).Mul(big.NewInt(user.Quota), big.NewInt(1_000_000_000/common.QuotaPerUSD)), big.NewInt(user.QuotaNanoRemainder))
}

func StaffDisableCustomer(actor, id int64, reason string, contexts ...context.Context) error {
	return adminRequestDB(contexts).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&User{}).Where("id = ? AND account_kind = ?", id, "customer").Updates(map[string]any{"status": common.StatusDisabled, "security_version": gorm.Expr("security_version + 1")})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrUserNotFound
		}
		return writeStaffAudit(tx, actor, id, "customer_disabled", reason, "")
	})
}

func StaffDisableCustomerKey(actor, owner, key int64, reason string, contexts ...context.Context) error {
	return adminRequestDB(contexts).Transaction(func(tx *gorm.DB) error {
		var user User
		if err := tx.Where("id = ? AND account_kind = ?", owner, "customer").First(&user).Error; err != nil {
			return err
		}
		res := tx.Model(&Token{}).Where("id = ? AND user_id = ?", key, owner).Updates(map[string]any{"status": common.StatusDisabled, "revision": gorm.Expr("revision + 1")})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrTokenNotFound
		}
		return writeStaffAudit(tx, actor, key, "customer_key_disabled", reason, "")
	})
}

// StaffAdjustBalance books the exact nanoUSD adjustment and its audit atomically.
// The sub-quota remainder preserves values below the legacy settlement quantum.
func StaffAdjustBalance(actor, id int64, amount int64, kind, reason, operation string, contexts ...context.Context) (*LedgerEntry, error) {
	if amount == 0 || strings.TrimSpace(reason) == "" {
		return nil, errors.New("nonzero amount and reason are required")
	}
	var result LedgerEntry
	key := "staff-adjust-" + big.NewInt(actor).String() + "-" + operation
	description := kind + " · " + reason
	err := adminRequestDB(contexts).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&User{}).Where("id = ?", id).UpdateColumn("id", gorm.Expr("id")).Error; err != nil {
			return err
		}
		if err := tx.Where("batch_key = ?", key).First(&result).Error; err == nil {
			if result.UserID != id || result.Amount != amount || result.Description != description {
				return errors.New("adjustment identifier was already used for different data")
			}
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var user User
		if err := tx.Where("id = ? AND account_kind = ?", id, "customer").First(&user).Error; err != nil {
			return err
		}
		beforeAmount := APIBalanceNano(&user).String()
		total := APIBalanceNano(&user)
		total.Add(total, big.NewInt(amount))
		if !total.IsInt64() {
			return errors.New("adjustment exceeds the ledger range")
		}
		var remainder big.Int
		quota, _ := new(big.Int).DivMod(total, big.NewInt(1_000_000_000/common.QuotaPerUSD), &remainder)
		if !quota.IsInt64() {
			return errors.New("adjustment exceeds the quota range")
		}
		if err := tx.Model(&user).Updates(map[string]any{"quota": quota.Int64(), "quota_nano_remainder": remainder.Int64()}).Error; err != nil {
			return err
		}
		result = LedgerEntry{UserID: id, Type: "adjustment", Amount: amount, BalanceAfter: total.Int64(), Description: description, FundingSource: FundingSourceStaffAdjustment, BatchKey: &key}
		if err := tx.Create(&result).Error; err != nil {
			return err
		}
		return writeStaffAudit(tx, actor, id, "balance_"+kind, reason, big.NewInt(amount).String(), map[string]string{"balance_nano": beforeAmount}, map[string]string{"balance_nano": total.String(), "amount_nano": big.NewInt(amount).String()})
	})
	return &result, err
}

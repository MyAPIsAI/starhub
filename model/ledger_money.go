package model

import (
	"errors"
	"math/big"
	"strings"
)

const (
	LedgerAccountWallet   = "wallet"
	LedgerAccountQuota    = "api_quota"
	LedgerUnitNano        = "usd_nano"
	LedgerUnitLegacyQuota = "quota_v1"
)

// LedgerMoney preserves stored history and exposes its exact monetary scale.
// Only the old administrator adjustment writer used quota-valued ledger rows;
// wallet closure adjustments already used nano-USD and must not be rescaled.
func LedgerMoney(entry LedgerEntry) (account, unit, amountNano, balanceNano string, err error) {
	account, unit = entry.Account, entry.AmountUnit
	if account == "" && unit == "" {
		account, unit = LedgerAccountWallet, LedgerUnitNano
		if entry.Type == "adjustment" && strings.HasPrefix(entry.Description, "by admin ") {
			account, unit = LedgerAccountQuota, LedgerUnitLegacyQuota
		}
	}
	if (account != LedgerAccountWallet && account != LedgerAccountQuota) || (unit != LedgerUnitNano && unit != LedgerUnitLegacyQuota) || (unit == LedgerUnitLegacyQuota && account != LedgerAccountQuota) {
		return "", "", "", "", errors.New("unsupported ledger monetary identity")
	}
	convert := func(value int64) string {
		amount := big.NewInt(value)
		if unit == LedgerUnitLegacyQuota {
			amount.Mul(amount, big.NewInt(2000))
		}
		return amount.String()
	}
	return account, unit, convert(entry.Amount), convert(entry.BalanceAfter), nil
}

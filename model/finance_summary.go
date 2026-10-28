package model

import (
	"context"
	"database/sql"
	"errors"
	"math/big"

	"gorm.io/gorm"
	"starhub/common"
)

// FinanceSummary keeps the two existing accounting systems separate. Money
// travels as decimal integer strings, including totals exceeding int64.
type FinanceSummary struct {
	Since                      int64  `json:"since"`
	Until                      int64  `json:"until"`
	WalletBalanceNano          string `json:"wallet_balance_nano"`
	PaidRechargeNano           string `json:"paid_recharge_nano"`
	PaidRechargeCount          int64  `json:"paid_recharge_count"`
	PaidOrdersWithoutTimestamp int64  `json:"paid_orders_without_timestamp"`
	LedgerCreditsNano          string `json:"ledger_credits_nano"`
	LedgerDebitsNano           string `json:"ledger_debits_nano"`
	APICollectedNano           string `json:"api_collected_nano"`
	APIUnfundedNano            string `json:"api_unfunded_nano"`
	APIRequestCount            int64  `json:"api_request_count"`
	APISuccessCount            int64  `json:"api_success_count"`
	APIFailedCount             int64  `json:"api_failed_count"`
	APICancelledCount          int64  `json:"api_cancelled_count"`
	WalletAndAPIQuotaLinked    bool   `json:"wallet_and_api_quota_linked"`
}

func readFinanceRows(query *gorm.DB, consume func(*sql.Rows) error) error {
	rows, err := query.Rows()
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := consume(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

// ReadFinanceSummary takes one SQLite read snapshot, without creating wallets,
// expiring orders, or changing the underlying accounting state.
func ReadFinanceSummary(ctx context.Context, since, until int64) (*FinanceSummary, error) {
	if since <= 0 || until <= since {
		return nil, errors.New("invalid finance summary time window")
	}
	result := &FinanceSummary{Since: since, Until: until}
	var wallet, paid, credits, debits, collected, unfunded, operand big.Int
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := readFinanceRows(tx.Model(&Wallet{}).Select("balance"), func(rows *sql.Rows) error {
			var amount int64
			if err := rows.Scan(&amount); err != nil {
				return err
			}
			operand.SetInt64(amount)
			wallet.Add(&wallet, &operand)
			return nil
		}); err != nil {
			return err
		}
		if err := readFinanceRows(tx.Model(&RechargeOrder{}).Select("amount").Where("status = ? AND paid_at >= ? AND paid_at < ?", RechargePaid, since, until), func(rows *sql.Rows) error {
			var amount int64
			if err := rows.Scan(&amount); err != nil {
				return err
			}
			operand.SetInt64(amount)
			paid.Add(&paid, &operand)
			result.PaidRechargeCount++
			return nil
		}); err != nil {
			return err
		}
		if err := tx.Model(&RechargeOrder{}).Where("status = ? AND paid_at <= 0", RechargePaid).Count(&result.PaidOrdersWithoutTimestamp).Error; err != nil {
			return err
		}
		if err := readFinanceRows(tx.Model(&LedgerEntry{}).Select("amount").Where("created_at >= ? AND created_at < ?", since, until), func(rows *sql.Rows) error {
			var amount int64
			if err := rows.Scan(&amount); err != nil {
				return err
			}
			operand.SetInt64(amount)
			if amount < 0 {
				debits.Sub(&debits, &operand)
			} else {
				credits.Add(&credits, &operand)
			}
			return nil
		}); err != nil {
			return err
		}
		return readFinanceRows(tx.Model(&Log{}).Select("quota, unfunded_quota, status").Where("created_at >= ? AND created_at < ?", since, until), func(rows *sql.Rows) error {
			var quota, shortfall int64
			var status int
			if err := rows.Scan(&quota, &shortfall, &status); err != nil {
				return err
			}
			operand.SetInt64(quota)
			collected.Add(&collected, &operand)
			operand.SetInt64(shortfall)
			unfunded.Add(&unfunded, &operand)
			result.APIRequestCount++
			switch status {
			case 1:
				result.APISuccessCount++
			case 2:
				result.APIFailedCount++
			case 3:
				result.APICancelledCount++
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	result.WalletBalanceNano = wallet.String()
	result.PaidRechargeNano = paid.String()
	result.LedgerCreditsNano = credits.String()
	result.LedgerDebitsNano = debits.String()
	// QuotaPerUSD is the authoritative API quota currency conversion. No
	// cash wallet movement is inferred from API consumption.
	toNano := func(quota *big.Int) string {
		var nano big.Int
		nano.Mul(quota, big.NewInt(1_000_000_000))
		nano.Quo(&nano, big.NewInt(common.QuotaPerUSD))
		return nano.String()
	}
	result.APICollectedNano = toNano(&collected)
	result.APIUnfundedNano = toNano(&unfunded)
	return result, nil
}

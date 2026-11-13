package model

import (
	"gorm.io/gorm"
	"math"
	"math/big"
	"starhub/common"
)

type usageMoney struct{ Charged, Unfunded big.Int }

// Monetary aggregation uses integer strings and arbitrary precision. SQLite
// SUM(int64) can overflow before conversion and cannot retain nano remainders.
func readGroupedLogMoney(query *gorm.DB, key string, args ...any) (map[string]*usageMoney, error) {
	rows, err := query.Select(key+" AS money_key, quota, quota_remainder_nano, unfunded_quota, unfunded_quota_remainder_nano", args...).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	totals := map[string]*usageMoney{}
	for rows.Next() {
		var group string
		var charged, chargedRemainder, unfunded, unfundedRemainder int64
		if err := rows.Scan(&group, &charged, &chargedRemainder, &unfunded, &unfundedRemainder); err != nil {
			return nil, err
		}
		total := totals[group]
		if total == nil {
			total = &usageMoney{}
			totals[group] = total
		}
		total.Charged.Add(&total.Charged, common.QuotaMoney(charged, chargedRemainder))
		total.Unfunded.Add(&total.Unfunded, common.QuotaMoney(unfunded, unfundedRemainder))
	}
	return totals, rows.Err()
}

func usageMoneyAt(totals map[string]*usageMoney, key string) *usageMoney {
	if total := totals[key]; total != nil {
		return total
	}
	return &usageMoney{}
}

// The legacy display projection saturates; canonical string totals never do.
func legacyUsageQuota(nano *big.Int) int64 {
	whole := new(big.Int).Quo(nano, big.NewInt(common.NanoPerLegacyQuota))
	if !whole.IsInt64() {
		if whole.Sign() < 0 {
			return math.MinInt64
		}
		return math.MaxInt64
	}
	return whole.Int64()
}

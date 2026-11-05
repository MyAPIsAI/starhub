package model

import (
	"context"
	"database/sql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"math/big"
	"starhub/common"
	"strconv"
	"time"
)

type ReconciliationDifference struct {
	Kind          string `json:"kind"`
	Expected      string `json:"expected_nano"`
	Actual        string `json:"actual_nano"`
	Delta         string `json:"delta_nano"`
	FundingSource string `json:"funding_source"`
}
type ReconciliationReport struct {
	ID                 int64                      `gorm:"primaryKey" json:"id"`
	Date               string                     `gorm:"size:10;uniqueIndex:idx_reconciliation_day,priority:1" json:"date"`
	UserID             int64                      `gorm:"uniqueIndex:idx_reconciliation_day,priority:2" json:"user_id"`
	Timezone           string                     `json:"timezone"`
	UsageNano          string                     `json:"usage_nano"`
	APILedgerNano      string                     `json:"api_ledger_nano"`
	OrderNano          string                     `json:"order_nano"`
	RechargeLedgerNano string                     `json:"recharge_ledger_nano"`
	Differences        []ReconciliationDifference `gorm:"serializer:json;type:text" json:"differences"`
	Provisional        bool                       `json:"provisional"`
	ActorID            int64                      `json:"actor_id"`
	CreatedAt          int64                      `gorm:"autoCreateTime" json:"created_at"`
}

func sumMoney(query *gorm.DB, column string) (*big.Int, error) {
	total := new(big.Int)
	rows, err := query.Select(column).Rows()
	if err != nil {
		return total, err
	}
	defer rows.Close()
	for rows.Next() {
		var n int64
		if err := rows.Scan(&n); err != nil {
			return total, err
		}
		total.Add(total, big.NewInt(n))
	}
	return total, rows.Err()
}
func RunDailyReconciliation(ctx context.Context, day string, userID, actor int64, now time.Time) ([]ReconciliationReport, error) {
	profile, err := CurrentDeploymentProfile(ctx)
	if err != nil {
		return nil, err
	}
	loc, err := time.LoadLocation(profile.Profile.Timezone)
	if err != nil {
		return nil, err
	}
	start, err := time.ParseInLocation("2006-01-02", day, loc)
	if err != nil || day > now.In(loc).Format("2006-01-02") {
		return nil, ErrAdminInvalid
	}
	end := start.AddDate(0, 0, 1)
	if err := FlushAPICharges(ctx, true); err != nil {
		return nil, err
	}
	users := []User{}
	q := DB.WithContext(ctx).Where("account_kind = ?", "customer")
	if userID > 0 {
		q = q.Where("id = ?", userID)
	}
	if err := q.Find(&users).Error; err != nil {
		return nil, err
	}
	reports := []ReconciliationReport{}
	for _, u := range users {
		report := ReconciliationReport{Date: day, UserID: u.ID, Timezone: loc.String(), ActorID: actor, CreatedAt: now.Unix(), Provisional: day == now.In(loc).Format("2006-01-02"), Differences: []ReconciliationDifference{}}
		err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			usage, err := sumMoney(tx.Model(&Log{}).Where("user_id = ? AND created_at >= ? AND created_at < ?", u.ID, start.Unix(), end.Unix()), "quota")
			if err != nil {
				return err
			}
			usage.Mul(usage, big.NewInt(1000000000/common.QuotaPerUSD))
			api, err := sumMoney(tx.Model(&LedgerEntry{}).Where("user_id = ? AND funding_source = ? AND created_at >= ? AND created_at < ?", u.ID, FundingSourceAPIQuota, start.Unix(), end.Unix()), "amount")
			if err != nil {
				return err
			}
			api.Neg(api)
			report.UsageNano = usage.String()
			report.APILedgerNano = api.String()
			difference := func(kind, source string, expected, actual *big.Int) {
				if expected.Cmp(actual) != 0 {
					report.Differences = append(report.Differences, ReconciliationDifference{Kind: kind, FundingSource: source, Expected: expected.String(), Actual: actual.String(), Delta: new(big.Int).Sub(actual, expected).String()})
				}
			}
			difference("api_usage_vs_ledger", FundingSourceAPIQuota, usage, api)
			ordersTotal, creditsTotal := new(big.Int), new(big.Int)
			sources := []string{}
			creditsSources := []string{}
			if err := tx.Model(&RechargeOrder{}).Where("user_id = ? AND status = ? AND paid_at >= ? AND paid_at < ?", u.ID, RechargePaid, start.Unix(), end.Unix()).Distinct().Pluck("funding_source", &sources).Error; err != nil {
				return err
			}
			if err := tx.Model(&LedgerEntry{}).Where("user_id = ? AND type = ? AND created_at >= ? AND created_at < ?", u.ID, "recharge", start.Unix(), end.Unix()).Distinct().Pluck("funding_source", &creditsSources).Error; err != nil {
				return err
			}
			seenSources := map[string]bool{}
			for _, source := range sources {
				seenSources[source] = true
			}
			for _, source := range creditsSources {
				if !seenSources[source] {
					sources = append(sources, source)
					seenSources[source] = true
				}
			}
			for _, source := range sources {
				orders, err := sumMoney(tx.Model(&RechargeOrder{}).Where("user_id = ? AND status = ? AND funding_source = ? AND paid_at >= ? AND paid_at < ?", u.ID, RechargePaid, source, start.Unix(), end.Unix()), "amount")
				if err != nil {
					return err
				}
				credits, err := sumMoney(tx.Model(&LedgerEntry{}).Where("user_id = ? AND type = ? AND funding_source = ? AND created_at >= ? AND created_at < ?", u.ID, "recharge", source, start.Unix(), end.Unix()), "amount")
				if err != nil {
					return err
				}
				difference("paid_orders_vs_ledger", source, orders, credits)
				ordersTotal.Add(ordersTotal, orders)
				creditsTotal.Add(creditsTotal, credits)
			}
			report.OrderNano = ordersTotal.String()
			report.RechargeLedgerNano = creditsTotal.String()
			return nil
		}, &sql.TxOptions{ReadOnly: true})
		if err != nil {
			return nil, err
		}
		reports = append(reports, report)
	}
	if len(reports) > 0 {
		if err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "date"}, {Name: "user_id"}}, DoUpdates: clause.AssignmentColumns([]string{"timezone", "usage_nano", "api_ledger_nano", "order_nano", "recharge_ledger_nano", "differences", "provisional", "actor_id", "created_at"})}).Create(&reports).Error; err != nil {
				return err
			}
			if actor > 0 {
				return RecordAdminChange(tx, actor, "reconciliation_run", "reconciliation", day, nil, map[string]any{"reports": len(reports), "user_id": userID}, "", "")
			}
			return nil
		}); err != nil {
			return nil, err
		}
	}
	return reports, nil
}
func ReadReconciliation(ctx context.Context, end string, days int, user int64) ([]ReconciliationReport, error) {
	if days < 1 || days > 31 {
		return nil, ErrAdminInvalid
	}
	at, err := time.Parse("2006-01-02", end)
	if err != nil {
		return nil, err
	}
	since := at.AddDate(0, 0, -days+1).Format("2006-01-02")
	rows := []ReconciliationReport{}
	q := DB.WithContext(ctx).Where("date >= ? AND date <= ?", since, end)
	if user > 0 {
		q = q.Where("user_id = ?", user)
	}
	if err := q.Order("id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	result := []ReconciliationReport{}
	for _, r := range rows {
		key := r.Date + ":" + strconv.FormatInt(r.UserID, 10)
		if !seen[key] {
			seen[key] = true
			result = append(result, r)
		}
	}
	return result, nil
}

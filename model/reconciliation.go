package model

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"math/big"
	"sort"
	"starhub/common"
	"strconv"
	"time"
)

type ReconciliationDifference struct {
	TokenID       int64  `json:"token_id,omitempty"`
	Reference     string `json:"reference,omitempty"`
	Kind          string `json:"kind"`
	Expected      string `json:"expected_nano"`
	Actual        string `json:"actual_nano"`
	Delta         string `json:"delta_nano"`
	FundingSource string `json:"funding_source"`
}
type ReconciliationReport struct {
	AdmissionTimeVerified bool                       `gorm:"not null;default:false" json:"admission_time_verified"`
	Username              string                     `json:"username"`
	BalanceExpectedNano   string                     `json:"balance_expected_nano"`
	BalanceActualNano     string                     `json:"balance_actual_nano"`
	ReservedNano          string                     `json:"reserved_nano"`
	ExternalStatus        string                     `json:"external_status"`
	ImportedSources       []string                   `gorm:"serializer:json;type:text" json:"imported_sources"`
	ChecksVersion         int                        `gorm:"not null;default:0" json:"checks_version"`
	AlertFingerprint      string                     `json:"-"`
	ID                    int64                      `gorm:"primaryKey" json:"id"`
	Date                  string                     `gorm:"size:10;uniqueIndex:idx_reconciliation_day,priority:1" json:"date"`
	UserID                int64                      `gorm:"uniqueIndex:idx_reconciliation_day,priority:2" json:"user_id"`
	Timezone              string                     `json:"timezone"`
	UsageNano             string                     `json:"usage_nano"`
	APILedgerNano         string                     `json:"api_ledger_nano"`
	OrderNano             string                     `json:"order_nano"`
	RechargeLedgerNano    string                     `json:"recharge_ledger_nano"`
	Differences           []ReconciliationDifference `gorm:"serializer:json;type:text" json:"differences"`
	Provisional           bool                       `json:"provisional"`
	ActorID               int64                      `json:"actor_id"`
	CreatedAt             int64                      `gorm:"autoCreateTime" json:"created_at"`
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
	reports := []ReconciliationReport{}
	// Flush, snapshot, reports and notification outbox share one writer transaction.
	// A concurrent settlement cannot appear in only one side of the comparison.
	err = DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := flushAPICharges(tx, true); err != nil {
			return err
		}
		users := []User{}
		q := tx.Where("account_kind = ?", "customer")
		if userID > 0 {
			q = q.Where("id = ?", userID)
		}
		if err := q.Order("id").Find(&users).Error; err != nil {
			return err
		}
		for _, u := range users {
			r := ReconciliationReport{Date: day, UserID: u.ID, Username: u.Username, Timezone: loc.String(), ActorID: actor, CreatedAt: now.Unix(), Provisional: day == now.In(loc).Format("2006-01-02"), Differences: []ReconciliationDifference{}, ChecksVersion: 2, ExternalStatus: "not_imported", ImportedSources: []string{}}
			usage, err := sumMoney(tx.Model(&Log{}).Where("user_id = ? AND created_at >= ? AND created_at < ?", u.ID, start.Unix(), end.Unix()), "quota")
			if err != nil {
				return err
			}
			usage.Mul(usage, big.NewInt(nanoPerQuota))
			api, err := sumMoney(tx.Model(&LedgerEntry{}).Where("user_id = ? AND funding_source = ? AND created_at >= ? AND created_at < ?", u.ID, FundingSourceAPIQuota, start.Unix(), end.Unix()), "amount")
			if err != nil {
				return err
			}
			api.Neg(api)
			r.UsageNano, r.APILedgerNano = usage.String(), api.String()
			var legacyTimes int64
			if err := tx.Model(&Log{}).Where("user_id = ? AND created_at >= ? AND created_at < ? AND accepted_at = 0", u.ID, start.Unix(), end.Unix()).Count(&legacyTimes).Error; err != nil {
				return err
			}
			r.AdmissionTimeVerified = legacyTimes == 0
			r.addDifference("api_usage_vs_ledger", FundingSourceAPIQuota, 0, "", usage, api)
			if err := reconcileBalances(tx, &r, u); err != nil {
				return err
			}
			if err := reconcileKeys(tx, &r, start.Unix(), end.Unix()); err != nil {
				return err
			}
			if err := reconcileInternalOrders(tx, &r, start.Unix(), end.Unix()); err != nil {
				return err
			}
			if err := reconcileImportedPayments(tx, &r, start.Unix(), end.Unix()); err != nil {
				return err
			}
			sort.Slice(r.Differences, func(i, j int) bool {
				a, b := r.Differences[i], r.Differences[j]
				return fmt.Sprintf("%s:%020d:%s:%s", a.Kind, a.TokenID, a.FundingSource, a.Reference) < fmt.Sprintf("%s:%020d:%s:%s", b.Kind, b.TokenID, b.FundingSource, b.Reference)
			})
			var previous ReconciliationReport
			if err := tx.Where("date = ? AND user_id = ?", day, u.ID).Limit(1).Find(&previous).Error; err != nil {
				return err
			}
			if len(r.Differences) > 0 {
				raw, _ := json.Marshal(r.Differences)
				r.AlertFingerprint = fmt.Sprintf("%x", sha256.Sum256(raw))
				if previous.AlertFingerprint != r.AlertFingerprint {
					if err := queueReconciliationAlert(tx, r, now); err != nil {
						return err
					}
				}
			}
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "date"}, {Name: "user_id"}}, DoUpdates: clause.AssignmentColumns([]string{"timezone", "username", "usage_nano", "api_ledger_nano", "order_nano", "recharge_ledger_nano", "differences", "provisional", "actor_id", "created_at", "balance_expected_nano", "balance_actual_nano", "reserved_nano", "external_status", "imported_sources", "checks_version", "alert_fingerprint", "admission_time_verified"})}).Create(&r).Error; err != nil {
				return err
			}
			if err := tx.Where("user_id = ? AND day = ? AND timezone = ?", u.ID, day, loc.String()).Delete(&ReconciliationDirtyDay{}).Error; err != nil {
				return err
			}
			reports = append(reports, r)
		}
		if actor > 0 {
			return RecordAdminChange(tx, actor, "reconciliation_run", "reconciliation", day, nil, map[string]any{"reports": len(reports), "user_id": userID}, "", "")
		}
		return nil
	})
	return reports, err
}

func (r *ReconciliationReport) addDifference(kind, source string, token int64, ref string, expected, actual *big.Int) {
	if expected.Cmp(actual) != 0 {
		r.Differences = append(r.Differences, ReconciliationDifference{Kind: kind, FundingSource: source, TokenID: token, Reference: ref, Expected: expected.String(), Actual: actual.String(), Delta: new(big.Int).Sub(actual, expected).String()})
	}
}

const nanoPerQuota = int64(1_000_000_000 / common.QuotaPerUSD)

func reconcileBalances(tx *gorm.DB, r *ReconciliationReport, u User) error {
	// API credit and the legacy simulated wallet are independent balances.
	sources := []string{FundingSourceAPIQuota, FundingSourceStaffAdjustment, FundingSourcePaymentProvider}
	expected, err := sumMoney(tx.Model(&LedgerEntry{}).Where("user_id = ? AND funding_source IN ?", u.ID, sources), "amount")
	if err != nil {
		return err
	}
	reserved := new(big.Int).Mul(big.NewInt(u.ReservedQuota), big.NewInt(nanoPerQuota))
	actual := new(big.Int).Add(APIBalanceNano(&u), reserved)
	r.BalanceExpectedNano, r.BalanceActualNano, r.ReservedNano = expected.String(), actual.String(), reserved.String()
	r.addDifference("balance_vs_ledger", FundingSourceAPIQuota, 0, "", expected, actual)
	if u.ReservedQuota < 0 {
		r.addDifference("invalid_reservation", FundingSourceAPIQuota, 0, "", new(big.Int), reserved)
	}
	walletExpected, err := sumMoney(tx.Model(&LedgerEntry{}).Where("user_id = ? AND funding_source NOT IN ?", u.ID, sources), "amount")
	if err != nil {
		return err
	}
	var wallet Wallet
	if err = tx.Where("user_id = ?", u.ID).Limit(1).Find(&wallet).Error; err != nil {
		return err
	}
	r.addDifference("wallet_vs_ledger", "legacy_wallet", 0, "", walletExpected, big.NewInt(wallet.Balance))
	return nil
}

func reconcileKeys(tx *gorm.DB, r *ReconciliationReport, since, until int64) error {
	ids := []int64{}
	if err := tx.Raw("SELECT token_id FROM logs WHERE user_id = ? UNION SELECT token_id FROM api_charge_events WHERE user_id = ? UNION SELECT id FROM tokens WHERE user_id = ?", r.UserID, r.UserID, r.UserID).Scan(&ids).Error; err != nil {
		return err
	}
	for _, id := range ids {
		if id == 0 {
			continue
		}
		daily, err := sumMoney(tx.Model(&Log{}).Where("user_id = ? AND token_id = ? AND created_at >= ? AND created_at < ?", r.UserID, id, since, until), "quota")
		if err != nil {
			return err
		}
		daily.Mul(daily, big.NewInt(nanoPerQuota))
		metered, err := sumMoney(tx.Model(&APIChargeEvent{}).Where("user_id = ? AND token_id = ? AND created_at >= ? AND created_at < ?", r.UserID, id, since, until), "amount")
		if err != nil {
			return err
		}
		r.addDifference("key_usage_vs_details", FundingSourceAPIQuota, id, "", daily, metered)
		var key struct {
			ID        int64
			UsedQuota int64
		}
		if err := tx.Table("tokens").Select("id", "used_quota").Where("id = ? AND user_id = ?", id, r.UserID).Limit(1).Find(&key).Error; err != nil {
			return err
		}
		if key.ID != 0 {
			total, err := sumMoney(tx.Model(&Log{}).Where("user_id = ? AND token_id = ?", r.UserID, id), "quota")
			if err != nil {
				return err
			}
			total.Mul(total, big.NewInt(nanoPerQuota))
			r.addDifference("key_counter_vs_details", FundingSourceAPIQuota, id, "", total, new(big.Int).Mul(big.NewInt(key.UsedQuota), big.NewInt(nanoPerQuota)))
		}
	}
	return nil
}

func reconcileInternalOrders(tx *gorm.DB, r *ReconciliationReport, since, until int64) error {
	var sources []string
	if err := tx.Raw("SELECT funding_source FROM recharge_orders WHERE user_id = ? AND status = ? AND paid_at >= ? AND paid_at < ? UNION SELECT funding_source FROM ledger_entries WHERE user_id = ? AND type = ? AND created_at >= ? AND created_at < ?", r.UserID, RechargePaid, since, until, r.UserID, "recharge", since, until).Scan(&sources).Error; err != nil {
		return err
	}
	ordersTotal, creditsTotal := new(big.Int), new(big.Int)
	for _, source := range sources {
		orders, err := sumMoney(tx.Model(&RechargeOrder{}).Where("user_id = ? AND status = ? AND funding_source = ? AND paid_at >= ? AND paid_at < ?", r.UserID, RechargePaid, source, since, until), "amount")
		if err != nil {
			return err
		}
		credits, err := sumMoney(tx.Model(&LedgerEntry{}).Where("user_id = ? AND type = ? AND funding_source = ? AND created_at >= ? AND created_at < ?", r.UserID, "recharge", source, since, until), "amount")
		if err != nil {
			return err
		}
		r.addDifference("paid_orders_vs_ledger", source, 0, "", orders, credits)
		ordersTotal.Add(ordersTotal, orders)
		creditsTotal.Add(creditsTotal, credits)
	}
	r.OrderNano, r.RechargeLedgerNano = ordersTotal.String(), creditsTotal.String()
	return nil
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

package model

import (
	"context"
	"database/sql"
	"errors"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"starhub/common"
)

type OperatingTotals struct {
	RechargeNano    string `json:"recharge_nano"`
	ConsumptionNano string `json:"consumption_nano"`
	Requests        int64  `json:"requests"`
	Successful      int64  `json:"successful"`
	Failed          int64  `json:"failed"`
	Cancelled       int64  `json:"cancelled"`
	ActiveAccounts  int64  `json:"active_accounts"`
	NewAccounts     int64  `json:"new_accounts"`
	PayingAccounts  int64  `json:"paying_accounts"`
}

type OperatingFunding struct {
	OfflineNano     string `json:"offline_nano"`
	SimulationNano  string `json:"simulation_nano"`
	UnverifiedNano  string `json:"unverified_nano"`
	OtherCreditNano string `json:"other_credit_nano"`
}

type OperatingBucket struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	AmountNano string `json:"amount_nano"`
	Requests   int64  `json:"requests"`
	amount     big.Int
}

type OperatingPoint struct {
	At         int64             `json:"at"`
	AmountNano string            `json:"amount_nano"`
	Groups     []OperatingBucket `json:"groups"`
}

type OperatingDashboard struct {
	Timezone       string            `json:"timezone"`
	RegionID       string            `json:"region_id"`
	StartDate      string            `json:"start_date"`
	EndDate        string            `json:"end_date"`
	Since          int64             `json:"since"`
	Until          int64             `json:"until"`
	DataUntil      int64             `json:"data_until"`
	UpdatedAt      int64             `json:"updated_at"`
	RefreshSeconds int               `json:"refresh_seconds"`
	Granularity    string            `json:"granularity"`
	Totals         OperatingTotals   `json:"totals"`
	Funding        OperatingFunding  `json:"funding"`
	Series         []OperatingPoint  `json:"series"`
	Groups         []OperatingBucket `json:"groups"`
	Vendors        []OperatingBucket `json:"vendors"`
	CallTypes      []OperatingBucket `json:"call_types"`
	Models         []OperatingBucket `json:"models"`
	TopAccounts    []OperatingBucket `json:"top_accounts"`
	TopModels      []OperatingBucket `json:"top_models"`
}

func addOperatingBucket(rows map[string]*OperatingBucket, id, name string, amount *big.Int) {
	row := rows[id]
	if row == nil {
		row = &OperatingBucket{ID: id, Name: name}
		rows[id] = row
	}
	row.amount.Add(&row.amount, amount)
	row.AmountNano = row.amount.String()
	row.Requests++
}

func sortedOperatingBuckets(rows map[string]*OperatingBucket) []OperatingBucket {
	result := make([]OperatingBucket, 0, len(rows))
	for _, row := range rows {
		result = append(result, *row)
	}
	sort.Slice(result, func(i, j int) bool {
		if order := result[i].amount.Cmp(&result[j].amount); order != 0 {
			return order > 0
		}
		return result[i].ID < result[j].ID
	})
	return result
}

// ReadOperatingDashboard reads a single durable snapshot. It never sums a
// process-local counter or modifies balances, orders, or audit records.
func ReadOperatingDashboard(ctx context.Context, start, until, now time.Time, region string) (*OperatingDashboard, error) {
	if !start.Before(until) || start.After(now) || until.After(start.AddDate(1, 0, 0)) {
		return nil, errors.New("invalid operating dashboard window")
	}
	loc := start.Location()
	end := min(until.Unix(), now.Unix()+1)
	result := &OperatingDashboard{Timezone: loc.String(), RegionID: region, StartDate: start.Format("2006-01-02"), EndDate: until.Add(-time.Second).In(loc).Format("2006-01-02"), Since: start.Unix(), Until: until.Unix(), DataUntil: end, UpdatedAt: now.Unix(), RefreshSeconds: 300, Granularity: "day", Series: []OperatingPoint{}}
	if until.Equal(start.AddDate(0, 0, 1)) || until.Sub(start) <= 24*time.Hour {
		result.Granularity = "hour"
	}
	groups, vendors, kinds, models, accounts := map[string]*OperatingBucket{}, map[string]*OperatingBucket{}, map[string]*OperatingBucket{}, map[string]*OperatingBucket{}, map[string]*OperatingBucket{}
	series := map[int64]map[string]*OperatingBucket{}
	active, paying := map[int64]bool{}, map[int64]bool{}
	var consumption, offline, simulation, unverified, other big.Int
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&User{}).Where("account_kind = 'customer' AND created_at >= ? AND created_at < ?", start.Unix(), end).Count(&result.Totals.NewAccounts).Error; err != nil {
			return err
		}
		groupNames, modelNames := map[string]string{}, map[string]string{}
		if err := readFinanceRows(tx.Model(&ServiceGroupConfig{}).Select("group_id, name"), func(rows *sql.Rows) error {
			var id, name string
			if err := rows.Scan(&id, &name); err != nil {
				return err
			}
			groupNames[id] = name
			return nil
		}); err != nil {
			return err
		}
		if err := readFinanceRows(tx.Model(&StandardModel{}).Select("model, name"), func(rows *sql.Rows) error {
			var id, name string
			if err := rows.Scan(&id, &name); err != nil {
				return err
			}
			modelNames[id] = name
			return nil
		}); err != nil {
			return err
		}
		query := tx.Table("logs AS l").Joins("JOIN users AS u ON u.id = l.user_id AND u.account_kind = 'customer'").Select("l.user_id, u.username, l.model, l.vendor, l.call_type, l.service_group, l.quota, l.status, l.created_at").Where("l.created_at >= ? AND l.created_at < ?", start.Unix(), end)
		if err := readFinanceRows(query, func(rows *sql.Rows) error {
			var user, quota, at int64
			var name, model, vendor, kind, group string
			var status int
			if err := rows.Scan(&user, &name, &model, &vendor, &kind, &group, &quota, &status, &at); err != nil {
				return err
			}
			amount := new(big.Int).Mul(big.NewInt(quota), big.NewInt(1_000_000_000/common.QuotaPerUSD))
			consumption.Add(&consumption, amount)
			result.Totals.Requests++
			switch status {
			case 1:
				result.Totals.Successful++
			case 2:
				result.Totals.Failed++
			case 3:
				result.Totals.Cancelled++
			}
			active[user] = true
			addOperatingBucket(groups, group, groupNames[group], amount)
			addOperatingBucket(vendors, vendor, vendor, amount)
			addOperatingBucket(kinds, kind, kind, amount)
			addOperatingBucket(models, model, modelNames[model], amount)
			addOperatingBucket(accounts, strconv.FormatInt(user, 10), name, amount)
			stamp := time.Unix(at, 0).In(loc)
			if result.Granularity == "hour" {
				stamp = start.Add(time.Duration((at-start.Unix())/3600) * time.Hour)
			} else {
				stamp = time.Date(stamp.Year(), stamp.Month(), stamp.Day(), 0, 0, 0, 0, loc)
			}
			if series[stamp.Unix()] == nil {
				series[stamp.Unix()] = map[string]*OperatingBucket{}
			}
			addOperatingBucket(series[stamp.Unix()], group, groupNames[group], amount)
			return nil
		}); err != nil {
			return err
		}
		// Funding provenance is preserved. Only the explicit staff receipt kind
		// represents a registered payment in the currently supported workflows.
		query = tx.Table("ledger_entries AS e").Joins("JOIN users AS u ON u.id = e.user_id AND u.account_kind = 'customer'").Select("e.user_id, e.amount, e.type, e.funding_source, e.description").Where("e.created_at >= ? AND e.created_at < ? AND e.amount > 0 AND e.type IN ('recharge','adjustment')", start.Unix(), end)
		return readFinanceRows(query, func(rows *sql.Rows) error {
			var user, amount int64
			var kind, source, description string
			if err := rows.Scan(&user, &amount, &kind, &source, &description); err != nil {
				return err
			}
			value := big.NewInt(amount)
			switch {
			case kind == "adjustment" && source == FundingSourceStaffAdjustment && strings.HasPrefix(description, "offline_payment · "):
				offline.Add(&offline, value)
				paying[user] = true
			case source == FundingSourceSimulation:
				simulation.Add(&simulation, value)
			case kind == "adjustment" && source == FundingSourceStaffAdjustment:
				other.Add(&other, value)
			default:
				unverified.Add(&unverified, value)
			}
			return nil
		})
	}, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	result.Totals.ConsumptionNano, result.Totals.RechargeNano = consumption.String(), offline.String()
	result.Totals.ActiveAccounts, result.Totals.PayingAccounts = int64(len(active)), int64(len(paying))
	result.Funding = OperatingFunding{OfflineNano: offline.String(), SimulationNano: simulation.String(), UnverifiedNano: unverified.String(), OtherCreditNano: other.String()}
	result.Groups, result.Vendors, result.CallTypes = sortedOperatingBuckets(groups), sortedOperatingBuckets(vendors), sortedOperatingBuckets(kinds)
	result.Models = sortedOperatingBuckets(models)
	result.TopModels = result.Models[:min(20, len(result.Models))]
	allAccounts := sortedOperatingBuckets(accounts)
	result.TopAccounts = allAccounts[:min(20, len(allAccounts))]
	// Calendar days, including DST transitions, define the period boundaries.
	for at := start; at.Before(until); {
		values := sortedOperatingBuckets(series[at.Unix()])
		total := new(big.Int)
		for i := range values {
			total.Add(total, &values[i].amount)
		}
		result.Series = append(result.Series, OperatingPoint{At: at.Unix(), AmountNano: total.String(), Groups: values})
		if result.Granularity == "hour" {
			at = at.Add(time.Hour)
		} else {
			at = at.AddDate(0, 0, 1)
		}
	}
	return result, nil
}

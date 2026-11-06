package model

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"time"

	"gorm.io/gorm"
	"starhub/common"
	"starhub/config"
)

// APIChargeEvent is written in the same transaction as the immutable usage log.
// LedgerID is the outbox acknowledgement, never a second balance debit.
type APIChargeEvent struct {
	LogID        int64 `gorm:"primaryKey;autoIncrement:false"`
	UserID       int64 `gorm:"index;not null"`
	TokenID      int64 `gorm:"not null"`
	Amount       int64 `gorm:"not null"`
	BalanceAfter int64 `gorm:"not null"`
	CreatedAt    int64 `gorm:"index;not null"`
	LedgerID     int64 `gorm:"index;not null;default:0"`
}

func queueAPICharge(tx *gorm.DB, entry *Log) error {
	if entry.Quota <= 0 {
		return nil
	}
	const factor = int64(1_000_000_000 / common.QuotaPerUSD)
	if entry.Quota > math.MaxInt64/factor {
		return errors.New("API charge exceeds the nano-dollar ledger range")
	}
	var owner User
	if err := tx.Select("id", "quota", "quota_nano_remainder").First(&owner, entry.UserID).Error; err != nil {
		return err
	}
	if owner.Quota > math.MaxInt64/factor || owner.Quota < math.MinInt64/factor {
		return errors.New("API balance exceeds the nano-dollar ledger range")
	}
	balance := APIBalanceNano(&owner)
	if !balance.IsInt64() {
		return errors.New("API balance exceeds the nano-dollar ledger range")
	}
	return tx.Create(&APIChargeEvent{LogID: entry.ID, UserID: entry.UserID, TokenID: entry.TokenID, Amount: entry.Quota * factor, BalanceAfter: balance.Int64(), CreatedAt: entry.CreatedAt}).Error
}

// FlushAPICharges atomically claims durable events into append-only minute batches.
// Reads may flush the open minute so visible usage and the statement agree immediately.
func FlushAPICharges(ctx context.Context, includeOpenMinute bool) error {
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return flushAPICharges(tx, includeOpenMinute) })
}

func flushAPICharges(tx *gorm.DB, includeOpenMinute bool) error {
	if err := tx.Model(&APIChargeEvent{}).Where("ledger_id = 0").UpdateColumn("ledger_id", gorm.Expr("ledger_id")).Error; err != nil {
		return err
	}
	q := tx.Where("ledger_id = 0")
	if !includeOpenMinute {
		q = q.Where("created_at < ?", time.Now().Unix()/60*60)
	}
	var events []APIChargeEvent
	if err := q.Order("log_id ASC").Find(&events).Error; err != nil {
		return err
	}
	batches := map[string][]APIChargeEvent{}
	var order []string
	for _, e := range events {
		key := fmt.Sprintf("%d:%d:%d", e.UserID, e.TokenID, e.CreatedAt/60)
		if _, ok := batches[key]; !ok {
			order = append(order, key)
		}
		batches[key] = append(batches[key], e)
	}
	for _, key := range order {
		items := batches[key]
		var amount int64
		ids := make([]int64, 0, len(items))
		for _, e := range items {
			if e.Amount > math.MaxInt64-amount {
				return errors.New("minute charge exceeds the ledger range")
			}
			amount += e.Amount
			ids = append(ids, e.LogID)
		}
		last := items[len(items)-1]
		batchKey := fmt.Sprintf("api:%s:%d:%d", key, items[0].LogID, last.LogID)
		entry := LedgerEntry{UserID: last.UserID, TokenID: last.TokenID, Type: "call_summary", Amount: -amount, BalanceAfter: last.BalanceAfter, Description: "API usage", RequestCount: int64(len(items)), FundingSource: FundingSourceAPIQuota, CreatedAt: last.CreatedAt, BatchKey: &batchKey}
		if err := tx.Create(&entry).Error; err != nil {
			return err
		}
		for start := 0; start < len(ids); start += 500 {
			end := min(start+500, len(ids))
			if err := tx.Model(&APIChargeEvent{}).Where("log_id IN ? AND ledger_id = 0", ids[start:end]).UpdateColumn("ledger_id", entry.ID).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// StatementLedger groups API batches by regional calendar day and key for display.
// Raw batches and source logs remain immutable and retain independent provenance.
func StatementLedger(userID int64, since, until int64, trace string) ([]LedgerEntry, error) {
	return statementLedger(userID, since, until, trace, nil)
}

// StatementLedgerForOrder is called only after the order's owner is authorized.
func StatementLedgerForOrder(order *RechargeOrder, trace string) ([]LedgerEntry, error) {
	return statementLedger(order.UserID, 0, 0, trace, []string{order.OrderNumber, strconv.FormatInt(order.ID, 10)})
}

func statementLedger(userID int64, since, until int64, trace string, orderRefs []string) ([]LedgerEntry, error) {
	if err := FlushAPICharges(context.Background(), true); err != nil {
		return nil, err
	}
	q := DB.Model(&LedgerEntry{})
	if userID > 0 {
		q = q.Where("user_id = ?", userID)
	}
	if len(orderRefs) > 0 {
		q = q.Where("order_id IN ?", orderRefs)
	}
	if since > 0 {
		q = q.Where("created_at >= ?", since)
	}
	if until > 0 {
		q = q.Where("created_at < ?", until)
	}
	if trace != "" {
		q = q.Where("trace_id = ? OR id IN (SELECT ledger_id FROM api_charge_events WHERE log_id IN (SELECT id FROM logs WHERE user_id = ? AND (trace_id = ? OR ('log_' || id) = ?)))", trace, userID, trace, trace)
	}
	var rows []LedgerEntry
	if err := q.Order("created_at DESC, id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	loc, _ := time.LoadLocation(config.RegionTimezone())
	if trace != "" {
		for i := range rows {
			rows[i].TraceID = trace
		}
	}
	return GroupLedgerByDay(rows, loc)
}

// GroupLedgerByDay is shared by the statement API and server-generated CSV files.
func GroupLedgerByDay(rows []LedgerEntry, loc *time.Location) ([]LedgerEntry, error) {
	result := make([]LedgerEntry, 0, len(rows))
	grouped := map[string]int{}
	for _, row := range rows {
		if row.FundingSource != FundingSourceAPIQuota {
			result = append(result, row)
			continue
		}
		day := time.Unix(row.CreatedAt, 0).In(loc).Format("2006-01-02")
		key := fmt.Sprintf("%s:%d", day, row.TokenID)
		if at, ok := grouped[key]; ok {
			if row.Amount < math.MinInt64-result[at].Amount {
				return nil, errors.New("daily charge exceeds the ledger range")
			}
			result[at].Amount += row.Amount
			result[at].RequestCount += row.RequestCount
		} else {
			grouped[key] = len(result)
			row.Description = fmt.Sprintf("API usage · %s · Key %d", day, row.TokenID)
			result = append(result, row)
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].CreatedAt > result[j].CreatedAt })
	return result, nil
}

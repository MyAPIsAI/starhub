package model

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"io"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Statements are immutable evidence. A new complete-day version supersedes
// the previous version without changing customer balances or payment status.
type PaymentStatement struct {
	ID         int64  `gorm:"primaryKey" json:"id"`
	Provider   string `gorm:"index" json:"provider"`
	Day        string `gorm:"index" json:"day"`
	Timezone   string `json:"timezone"`
	Source     string `json:"source"`
	Digest     string `gorm:"uniqueIndex" json:"digest"`
	Supersedes int64  `json:"supersedes"`
	RowCount   int    `json:"row_count"`
	TotalNano  string `json:"total_nano"`
	ActorID    int64  `json:"actor_id"`
	CreatedAt  int64  `json:"created_at"`
}

type PaymentStatementRow struct {
	ID              int64  `gorm:"primaryKey" json:"-"`
	StatementID     int64  `gorm:"index" json:"-"`
	ProviderOrderID string `gorm:"index" json:"provider_order_id"`
	PlatformOrderID string `json:"platform_order_id"`
	UserID          int64  `gorm:"index" json:"user_id"`
	AmountNano      int64  `json:"-"`
	PaidAt          int64  `json:"paid_at"`
}

type PaymentStatementImport struct {
	Provider           string `json:"provider"`
	Day                string `json:"day"`
	Source             string `json:"source"`
	CSV                string `json:"csv"`
	ExpectedPreviousID int64  `json:"expected_previous_id"`
}

var statementProviderPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

func ImportPaymentStatement(ctx context.Context, input PaymentStatementImport, actor int64, now time.Time) (*PaymentStatement, error) {
	profile, err := CurrentDeploymentProfile(ctx)
	if err != nil {
		return nil, err
	}
	loc, err := time.LoadLocation(profile.Profile.Timezone)
	if err != nil {
		return nil, err
	}
	day, err := time.ParseInLocation("2006-01-02", input.Day, loc)
	if err != nil || input.Day > now.In(loc).Format("2006-01-02") || !statementProviderPattern.MatchString(input.Provider) || strings.TrimSpace(input.Source) == "" || len(input.Source) > 200 || len(input.CSV) > 2*1024*1024 {
		return nil, fmt.Errorf("%w: invalid statement metadata", ErrAdminInvalid)
	}
	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(input.CSV, "\ufeff")))
	header, err := reader.Read()
	expected := []string{"provider_order_id", "platform_order_id", "user_id", "amount_cents", "currency", "paid_at"}
	if err != nil || strings.Join(header, ",") != strings.Join(expected, ",") {
		return nil, fmt.Errorf("%w: expected CSV columns %s", ErrAdminInvalid, strings.Join(expected, ","))
	}
	rows := []PaymentStatementRow{}
	seen := map[string]bool{}
	platformSeen := map[string]bool{}
	total := new(big.Int)
	for line := 2; ; line++ {
		cells, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil || len(rows) >= 10000 {
			return nil, fmt.Errorf("%w: invalid CSV row %d", ErrAdminInvalid, line)
		}
		user, e1 := strconv.ParseInt(cells[2], 10, 64)
		cents, e2 := strconv.ParseInt(cells[3], 10, 64)
		paid, e3 := time.Parse(time.RFC3339, cells[5])
		if e1 != nil || e2 != nil || e3 != nil || user <= 0 || cents <= 0 || cents > math.MaxInt64/10_000_000 || cells[4] != "USD" || cells[0] == "" || len(cells[0]) > 128 || len(cells[1]) > 128 || seen[cells[0]] || (cells[1] != "" && platformSeen[cells[1]]) || paid.Before(day) || !paid.Before(day.AddDate(0, 0, 1)) || paid.After(now) {
			return nil, fmt.Errorf("%w: invalid or duplicate payment at CSV row %d", ErrAdminInvalid, line)
		}
		seen[cells[0]] = true
		if cells[1] != "" {
			platformSeen[cells[1]] = true
		}
		amount := cents * 10_000_000
		rows = append(rows, PaymentStatementRow{ProviderOrderID: cells[0], PlatformOrderID: cells[1], UserID: user, AmountNano: amount, PaidAt: paid.Unix()})
		total.Add(total, big.NewInt(amount))
	}
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(input.Provider+"\n"+input.Day+"\n"+loc.String()+"\n"+input.CSV)))
	result := PaymentStatement{Provider: input.Provider, Day: input.Day, Timezone: loc.String(), Source: strings.TrimSpace(input.Source), Digest: digest, RowCount: len(rows), TotalNano: total.String(), ActorID: actor, CreatedAt: now.Unix()}
	err = DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Obtain the writer lock before the version and duplicate checks.
		if err := tx.Model(&DeploymentProfileState{}).Where("id = 1").UpdateColumn("id", gorm.Expr("id")).Error; err != nil {
			return err
		}
		var duplicate PaymentStatement
		if err := tx.Where("digest = ?", digest).Limit(1).Find(&duplicate).Error; err != nil {
			return err
		}
		if duplicate.ID != 0 {
			result = duplicate
			return nil
		}
		var previous PaymentStatement
		if err := tx.Where("provider = ? AND day = ?", input.Provider, input.Day).Order("id DESC").Limit(1).Find(&previous).Error; err != nil {
			return err
		}
		if previous.ID != input.ExpectedPreviousID {
			return fmt.Errorf("%w: statement version changed; reload before importing", ErrAdminConflict)
		}
		result.Supersedes = previous.ID
		for _, row := range rows {
			var count int64
			if err := tx.Model(&User{}).Where("id = ? AND account_kind = ?", row.UserID, "customer").Count(&count).Error; err != nil {
				return err
			}
			if count != 1 {
				return fmt.Errorf("%w: statement account does not exist", ErrAdminInvalid)
			}
			if err := tx.Table("payment_statement_rows AS r").Joins("JOIN payment_statements s ON s.id = r.statement_id").Where("s.provider = ? AND s.day <> ? AND r.provider_order_id = ?", input.Provider, input.Day, row.ProviderOrderID).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return fmt.Errorf("%w: provider order already appears on another day", ErrAdminInvalid)
			}
		}
		if err := tx.Create(&result).Error; err != nil {
			return err
		}
		for i := range rows {
			rows[i].StatementID = result.ID
		}
		if len(rows) > 0 {
			if err := tx.CreateInBatches(&rows, 100).Error; err != nil {
				return err
			}
		}
		// Include removed rows and unmatched platform credits on a replacement.
		var users []int64
		if err := tx.Model(&User{}).Where("account_kind = ?", "customer").Pluck("id", &users).Error; err != nil {
			return err
		}
		for _, id := range users {
			if err := markReconciliationDirty(tx, id, day.Unix()); err != nil {
				return err
			}
		}
		return RecordAdminChange(tx, actor, "payment_statement_imported", "payment_statement", strconv.FormatInt(result.ID, 10), nil, map[string]any{"provider": result.Provider, "day": result.Day, "rows": len(rows), "total_nano": result.TotalNano, "supersedes": result.Supersedes, "digest": result.Digest}, "", "")
	})
	return &result, err
}

func reconcileImportedPayments(tx *gorm.DB, r *ReconciliationReport, since, until int64) error {
	var batches []PaymentStatement
	if err := tx.Where("id IN (?)", tx.Model(&PaymentStatement{}).Select("MAX(id)").Where("day = ?", r.Date).Group("provider")).Order("provider").Find(&batches).Error; err != nil {
		return err
	}
	if len(batches) == 0 {
		return nil
	}
	r.ExternalStatus = "imported_scope"
	seen := map[string]bool{}
	for _, batch := range batches {
		if batch.Timezone != r.Timezone {
			r.ExternalStatus = "timezone_changed"
			continue
		}
		r.ImportedSources = append(r.ImportedSources, fmt.Sprintf("%s · %s · #%d", batch.Provider, batch.Source, batch.ID))
		var rows []PaymentStatementRow
		if err := tx.Where("statement_id = ? AND user_id = ?", batch.ID, r.UserID).Order("id").Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			actual := new(big.Int)
			if row.PlatformOrderID != "" {
				if seen[row.PlatformOrderID] {
					r.addDifference("duplicate_provider_mapping", batch.Provider, 0, row.ProviderOrderID, big.NewInt(row.AmountNano), new(big.Int))
					continue
				}
				seen[row.PlatformOrderID] = true
				var err error
				actual, err = sumMoney(tx.Model(&LedgerEntry{}).Where("user_id = ? AND type = ? AND funding_source = ? AND order_id = ? AND created_at >= ? AND created_at < ?", r.UserID, "recharge", FundingSourcePaymentProvider, row.PlatformOrderID, since, until), "amount")
				if err != nil {
					return err
				}
			}
			r.addDifference("provider_order_vs_ledger", batch.Provider, 0, row.ProviderOrderID, big.NewInt(row.AmountNano), actual)
		}
	}
	var credits []LedgerEntry
	if err := tx.Where("user_id = ? AND type = ? AND funding_source = ? AND created_at >= ? AND created_at < ?", r.UserID, "recharge", FundingSourcePaymentProvider, since, until).Find(&credits).Error; err != nil {
		return err
	}
	for _, credit := range credits {
		if !seen[credit.OrderID] {
			r.addDifference("ledger_without_provider_order", FundingSourcePaymentProvider, 0, credit.OrderID, new(big.Int), big.NewInt(credit.Amount))
		}
	}
	return nil
}

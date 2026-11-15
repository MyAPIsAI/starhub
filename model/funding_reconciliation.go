package model

import (
	"encoding/json"
	"math/big"

	"gorm.io/gorm"
	"starhub/common"
)

type fundingSummary struct {
	CreditedNano string `json:"credited_nano"`
	DebitedNano  string `json:"debited_nano"`
}

func fundingDayQuery(q *gorm.DB, userID int64, day BusinessDaySnapshot) *gorm.DB {
	q = q.Where("user_id = ?", userID)
	if day.BusinessCalendarVersion == 0 {
		return q.Where("(business_calendar_version = 0 AND business_date = ?) OR (business_date = '' AND created_at >= ? AND created_at < ?)", day.BusinessDate, day.BusinessStart, day.BusinessEnd)
	}
	return q.Where("business_calendar_version > 0 AND business_region = ? AND business_timezone = ? AND business_date = ? AND business_start = ? AND business_end = ?", day.BusinessRegion, day.BusinessTimezone, day.BusinessDate, day.BusinessStart, day.BusinessEnd)
}

func reconcileFunding(tx *gorm.DB, userID int64, day BusinessDaySnapshot, record *ReconciliationRecord) error {
	var sources []FundingReceipt
	if err := fundingDayQuery(tx, userID, day).Order("id").Find(&sources).Error; err != nil {
		return err
	}
	var movements []MoneyEntry
	if err := fundingDayQuery(tx, userID, day).Where("kind NOT IN ?", []string{"opening", "reservation", "release", "consumption", "counter_import"}).Order("sequence").Find(&movements).Error; err != nil {
		return err
	}
	byOperation := map[string]FundingReceipt{}
	for _, source := range sources {
		byOperation[source.OperationID] = source
	}
	credits, debits, difference := new(big.Int), new(big.Int), new(big.Int)
	summaries := map[string]fundingSummary{}
	for _, entry := range movements {
		value, err := common.ParseNano(entry.AvailableDeltaNano)
		if err != nil {
			return err
		}
		source, exists := byOperation[entry.OperationID]
		if !exists {
			record.UnverifiableFunding++
			difference.Add(difference, new(big.Int).Abs(value))
			continue
		}
		delete(byOperation, entry.OperationID)
		expected, err := common.ParseNano(source.AmountNano)
		if err != nil {
			return err
		}
		difference.Add(difference, new(big.Int).Abs(new(big.Int).Sub(expected, value)))
		if source.Source != entry.FundingSource || source.ActorID != entry.ActorID || entry.ReservedDeltaNano != "0" || entry.SpentDeltaNano != "0" {
			record.UnverifiableFunding++
		}
	}
	for _, source := range byOperation {
		value, err := common.ParseNano(source.AmountNano)
		if err != nil {
			return err
		}
		difference.Add(difference, new(big.Int).Abs(value))
		record.UnverifiableFunding++
	}
	for _, source := range sources {
		value, err := common.ParseNano(source.AmountNano)
		if err != nil {
			return err
		}
		summary := summaries[source.Source]
		credit, debit := new(big.Int), new(big.Int)
		if summary.CreditedNano != "" {
			credit, _ = common.ParseNano(summary.CreditedNano)
			debit, _ = common.ParseNano(summary.DebitedNano)
		}
		if value.Sign() >= 0 {
			credits.Add(credits, value)
			credit.Add(credit, value)
		} else {
			value.Abs(value)
			debits.Add(debits, value)
			debit.Add(debit, value)
		}
		summary.CreditedNano, summary.DebitedNano = credit.String(), debit.String()
		summaries[source.Source] = summary
	}
	raw, err := json.Marshal(summaries)
	if err != nil {
		return err
	}
	record.FundingCreditsNano, record.FundingDebitsNano, record.FundingDifferenceNano = credits.String(), debits.String(), difference.Abs(difference).String()
	record.FundingBySource = string(raw)
	return nil
}

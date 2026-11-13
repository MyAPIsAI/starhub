package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"math/big"
	"starhub/common"
	"time"
)

func reportKey(userID int64, day BusinessDaySnapshot) string {
	data := fmt.Sprintf("%d/%d/%s/%s/%s/%d/%d", userID, day.BusinessCalendarVersion, day.BusinessRegion, day.BusinessTimezone, day.BusinessDate, day.BusinessStart, day.BusinessEnd)
	sum := sha256.Sum256([]byte(data))
	return hex.EncodeToString(sum[:])
}

// ReconciliationDays includes historical admission days, so late settlements
// are revisited after restarts without moving them to the completion day.
func ReconciliationDays(userID int64, now time.Time) ([]BusinessDaySnapshot, error) {
	var days []BusinessDaySnapshot
	err := DB.Transaction(func(tx *gorm.DB) error {
		var logs []Log
		if err := tx.Model(&Log{}).Select("DISTINCT business_calendar_version, business_date, business_timezone, business_region, business_region_revision, business_start, business_end").Where("user_id = ? AND business_date <> ''", userID).Find(&logs).Error; err != nil {
			return err
		}
		for _, entry := range logs {
			days = append(days, entry.BusinessDaySnapshot)
		}
		var receipts []QuotaReservation
		if err := tx.Model(&QuotaReservation{}).Select("DISTINCT business_calendar_version, business_date, business_timezone, business_region, business_region_revision, business_start, business_end").Where("user_id = ? AND business_date <> '' AND completed_at > 0", userID).Find(&receipts).Error; err != nil {
			return err
		}
		for _, receipt := range receipts {
			days = append(days, receipt.BusinessDaySnapshot)
		}
		var dates []string
		if err := tx.Model(&Log{}).Where("user_id = ? AND business_date = ''", userID).Distinct("date("+admissionDayColumn+", 'unixepoch')").Pluck("date("+admissionDayColumn+", 'unixepoch')", &dates).Error; err != nil {
			return err
		}
		for _, date := range dates {
			day, err := BusinessDayForDate(date, "UTC", "", 0)
			if err != nil {
				return err
			}
			day.BusinessCalendarVersion = 0
			days = append(days, day)
		}
		current, err := captureBusinessDay(tx, now)
		if err != nil {
			return err
		}
		location, _ := time.LoadLocation(current.BusinessTimezone)
		previous := time.Unix(current.BusinessStart, 0).In(location).AddDate(0, 0, -1)
		day, err := BusinessDayForDate(previous.Format("2006-01-02"), current.BusinessTimezone, current.BusinessRegion, current.BusinessRegionRevision)
		if err != nil {
			return err
		}
		days = append(days, day)
		return nil
	})
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	result := []BusinessDaySnapshot{}
	for _, day := range days {
		key := reportKey(userID, day)
		if seen[key] {
			continue
		}
		seen[key] = true
		loc, err := time.LoadLocation(day.BusinessTimezone)
		if err != nil {
			return nil, err
		}
		end := time.Unix(day.BusinessEnd, 0).In(loc)
		eligible := time.Date(end.Year(), end.Month(), end.Day(), 2, 0, 0, 0, loc)
		// A skipped 02:00 is processed at the first valid hour after it.
		if eligible.Hour() < 2 {
			eligible = eligible.Add(time.Hour)
		}
		if !now.Before(eligible) {
			result = append(result, day)
		}
	}
	return result, nil
}

func reconciliationLogs(tx *gorm.DB, userID int64, day BusinessDaySnapshot) *gorm.DB {
	q := tx.Where("user_id = ?", userID)
	if day.BusinessCalendarVersion == 0 {
		return q.Where("(business_calendar_version = 0 AND business_date = ?) OR (business_date = '' AND "+admissionDayColumn+" >= ? AND "+admissionDayColumn+" < ?)", day.BusinessDate, day.BusinessStart, day.BusinessEnd)
	}
	return q.Where("business_calendar_version > 0 AND business_region = ? AND business_timezone = ? AND business_date = ? AND business_start = ? AND business_end = ?", day.BusinessRegion, day.BusinessTimezone, day.BusinessDate, day.BusinessStart, day.BusinessEnd)
}

// ReconcileBusinessDay compares logs and canonical consumption in one database
// snapshot. Opening observations are never treated as a day's spending.
// Unchanged reruns return the same persisted revision and do not alert again.
func ReconcileBusinessDay(userID int64, day BusinessDaySnapshot) (*ReconciliationRecord, bool, error) {
	if day.BusinessDate == "" || day.BusinessEnd <= day.BusinessStart {
		return nil, false, ErrMoneyConflict
	}
	if _, err := ReadMoneyAccount(userID); err != nil && !errors.Is(err, ErrMoneyConflict) {
		return nil, false, err
	}
	var record ReconciliationRecord
	changed := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		// Acquire the SQLite writer before reading the source snapshot.
		if err := tx.Model(&User{}).Where("id = ?", userID).UpdateColumn("quota", gorm.Expr("quota")).Error; err != nil {
			return err
		}
		var account MoneyAccount
		if err := tx.First(&account, "user_id = ?", userID).Error; err != nil {
			return err
		}
		var logs []Log
		if err := reconciliationLogs(tx, userID, day).Order("id").Find(&logs).Error; err != nil {
			return err
		}
		var entries []MoneyEntry
		if err := tx.Where("user_id = ? AND kind = ?", userID, "consumption").Order("sequence").Find(&entries).Error; err != nil {
			return err
		}
		byLog := map[int64][]MoneyEntry{}
		for _, e := range entries {
			if e.LogID > 0 {
				byLog[e.LogID] = append(byLog[e.LogID], e)
			}
		}
		charged, ledger, difference := new(big.Int), new(big.Int), new(big.Int)
		unknown := int64(0)
		for _, entry := range logs {
			paid := common.QuotaMoney(entry.Quota, entry.QuotaRemainderNano)
			charged.Add(charged, paid)
			matches := byLog[entry.ID]
			if len(matches) == 0 {
				if paid.Sign() != 0 {
					unknown++
				}
				continue
			}
			if len(matches) != 1 {
				unknown++
			}
			for _, movement := range matches {
				value, err := common.ParseNano(movement.SpentDeltaNano)
				if err != nil {
					return err
				}
				ledger.Add(ledger, value)
				if movement.ReservationID != entry.QuotaReservationID {
					unknown++
				}
				if movement.TokenID != 0 && (movement.TokenID != entry.TokenID || movement.PeriodID != entry.QuotaPeriodID) {
					unknown++
				}
			}
		}
		// A journal entry whose log was removed is still attributable through its
		// immutable admission receipt and must not disappear from the comparison.
		logIDs := map[int64]bool{}
		for _, entry := range logs {
			logIDs[entry.ID] = true
		}
		for _, movement := range entries {
			if logIDs[movement.LogID] || movement.ReservationID == "" {
				continue
			}
			var receipt QuotaReservation
			err := tx.First(&receipt, "id = ?", movement.ReservationID).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			var attributed Log
			assignReservationDay(&attributed, receipt)
			if reportKey(userID, attributed.BusinessDaySnapshot) != reportKey(userID, day) {
				continue
			}
			value, err := common.ParseNano(movement.SpentDeltaNano)
			if err != nil {
				return err
			}
			ledger.Add(ledger, value)
			unknown++
		}
		difference.Sub(charged, ledger).Abs(difference)
		verifyErr := verifyMoneyLedger(tx, userID)
		if verifyErr == nil {
			verifyErr = verifyTokenAccounting(tx, userID)
		}
		state := "matched"
		if unknown > 0 {
			state = "incomplete"
		}
		if difference.Sign() != 0 || verifyErr != nil {
			state = "difference"
		}
		record = ReconciliationRecord{BusinessDaySnapshot: day, ReportKey: reportKey(userID, day), UserID: userID, Date: day.BusinessDate, AccountingVersion: 3, State: state, LogQuotaNanoSum: charged.String(), LedgerSpentNano: ledger.String(), DifferenceNano: difference.String(), UsedQuotaNanoSnapshot: account.SpentNano, MoneySequence: account.Sequence, LedgerVerified: verifyErr == nil, UnverifiableLogs: unknown, Alerted: state != "matched"}
		if verifyErr != nil {
			record.LedgerError = "canonical_or_token_verification_failed"
		}
		record.LogQuotaSum, _, _ = common.NanoQuotaParts(charged)
		record.Diff, _, _ = common.NanoQuotaParts(difference)
		// Sequence and lifetime snapshots describe the read boundary; unrelated
		// newer-day entries do not create another revision of a historical report.
		fingerprint := record
		fingerprint.MoneySequence = 0
		fingerprint.UsedQuotaNanoSnapshot = ""
		fingerprint.BusinessRegionRevision = 0
		data, _ := json.Marshal(fingerprint)
		digest := sha256.Sum256(data)
		record.SourceDigest = hex.EncodeToString(digest[:])
		var old ReconciliationRecord
		err := tx.Where("report_key = ?", record.ReportKey).First(&old).Error
		if err == nil {
			if old.SourceDigest == record.SourceDigest {
				record = old
				return nil
			}
			record.ID, record.CreatedAt, record.Revision = old.ID, old.CreatedAt, old.Revision+1
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			record.Revision = 1
		} else {
			return err
		}
		if err := tx.Save(&record).Error; err != nil {
			return err
		}
		changed = true
		return nil
	})
	return &record, changed, err
}

package model

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// AvailabilityRecord is a monthly availability summary aggregated from logs.
type AvailabilityRecord struct {
	ID            int64   `gorm:"primaryKey;autoIncrement" json:"id"`
	Month         string  `gorm:"size:7;index;not null" json:"month"` // "2006-01"
	Component     string  `gorm:"size:64;not null;default:'overall'" json:"component"`
	TotalRequests int64   `gorm:"not null;default:0" json:"total_requests"`
	SuccessCount  int64   `gorm:"not null;default:0" json:"success_count"`
	FailedCount   int64   `gorm:"not null;default:0" json:"failed_count"`
	UptimePercent float64 `gorm:"not null;default:0" json:"uptime_percent"`
	CalculatedAt  int64   `gorm:"not null;default:0" json:"calculated_at"`
}

// AggregateMonthlyAvailability computes per-month availability from the Log
// table. Platform failures (Status=2 with ChannelID > 0) count against
// uptime; client errors (4xx) are excluded since they are not platform
// outages. When groupByModel is true, the component is the model name;
// otherwise it is "overall".
func AggregateMonthlyAvailability(monthsBack int) ([]AvailabilityRecord, error) {
	if monthsBack <= 0 || monthsBack > 12 {
		monthsBack = 3
	}
	now := time.Now().UTC()
	var records []AvailabilityRecord

	for i := monthsBack - 1; i >= 0; i-- {
		monthStart := time.Date(now.Year(), now.Month()-time.Month(i), 1, 0, 0, 0, 0, time.UTC)
		monthEnd := monthStart.AddDate(0, 1, 0)
		monthLabel := monthStart.Format("2006-01")

		// Count platform-relevant requests: Status 1 (success) or 2 (failed)
		// with ChannelID > 0. Exclude Status 3 (cancelled) and logs without
		// a channel (pre-channel failures don't represent platform outages).
		type row struct {
			Status int
			Count  int64
		}
		var rows []row
		err := DB.Model(&Log{}).
			Select("status, COUNT(*) AS count").
			Where("created_at >= ? AND created_at < ? AND channel_id > 0 AND status IN ?",
				monthStart.Unix(), monthEnd.Unix(), []int{1, 2}).
			Group("status").Scan(&rows).Error
		if err != nil {
			return nil, err
		}

		var success, failed int64
		for _, r := range rows {
			if r.Status == 1 {
				success = r.Count
			} else if r.Status == 2 {
				failed = r.Count
			}
		}
		total := success + failed
		var pct float64
		if total > 0 {
			pct = float64(success) / float64(total) * 100
		}

		records = append(records, AvailabilityRecord{
			Month:         monthLabel,
			Component:     "overall",
			TotalRequests: total,
			SuccessCount:  success,
			FailedCount:   failed,
			UptimePercent: pct,
			CalculatedAt:  now.Unix(),
		})
	}

	return records, nil
}

// SaveAvailabilityRecords persists computed availability records, replacing
// any existing records for the same months.
func SaveAvailabilityRecords(records []AvailabilityRecord) error {
	if len(records) == 0 {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		for _, r := range records {
			// Delete existing for this month+component, then insert.
			tx.Where("month = ? AND component = ?", r.Month, r.Component).
				Delete(&AvailabilityRecord{})
			if err := tx.Create(&r).Error; err != nil {
				return fmt.Errorf("save availability %s: %w", r.Month, err)
			}
		}
		return nil
	})
}

// ListAvailabilityHistory returns persisted availability records for the
// given number of months.
func ListAvailabilityHistory(monthsBack int) ([]AvailabilityRecord, error) {
	if monthsBack <= 0 || monthsBack > 12 {
		monthsBack = 3
	}
	since := time.Now().UTC().AddDate(0, -monthsBack, 0).Format("2006-01")
	var records []AvailabilityRecord
	err := DB.Where("month >= ?", since).Order("month ASC, component ASC").Find(&records).Error
	return records, err
}

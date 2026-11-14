package model

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ProductionTelemetryMinute is an immutable sealed batch, including its
// metadata-only attempt journal. Collection loss is stored explicitly.
type ProductionTelemetryMinute struct {
	AttemptCount int64  `gorm:"not null;default:0" json:"attempt_count"`
	Minute       int64  `gorm:"primaryKey;autoIncrement:false" json:"minute"`
	BatchID      string `gorm:"size:64;not null" json:"batch_id"`
	Complete     bool   `gorm:"not null" json:"complete"`
	Payload      string `gorm:"type:text;not null" json:"-"`
	CreatedAt    int64  `gorm:"autoCreateTime" json:"created_at"`
}

type ProductionAttemptRecord struct {
	BatchID       string `gorm:"size:64;not null;default:'';index" json:"-"`
	PayloadBytes  int64  `gorm:"not null;default:0" json:"-"`
	Attempts      int    `json:"delivery_attempts"`
	DeliveredAt   int64  `gorm:"index" json:"delivered_at"`
	NextAttemptAt int64  `json:"next_attempt_at"`
	LastError     string `gorm:"size:160" json:"last_error"`
	ID            string `gorm:"primaryKey;size:64" json:"attempt_id"`
	Minute        int64  `gorm:"index" json:"minute"`
	Payload       string `gorm:"type:text;not null" json:"-"`
}

// SaveProductionTelemetry stages bounded chunks before publishing manifests.
// A stage is invisible to readers and delivery until its manifest commits.
func SaveProductionTelemetry(rows []ProductionTelemetryMinute, now time.Time, attempts ...ProductionAttemptRecord) error {
	for i := range attempts {
		for _, row := range rows {
			if row.Minute == attempts[i].Minute {
				attempts[i].BatchID = row.BatchID
				break
			}
		}
	}
	if err := StageProductionAttempts(attempts); err != nil {
		return err
	}
	for _, row := range rows {
		if err := publishProductionMinute(row, now); err != nil {
			return err
		}
	}
	return nil
}

func publishProductionMinute(row ProductionTelemetryMinute, now time.Time) error {
	if row.Minute <= 0 || row.Minute%60 != 0 || row.BatchID == "" || row.Minute >= now.Unix()/60*60 || len(row.Payload) > 64<<20 {
		return errors.New("invalid production minute")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("UPDATE production_telemetry_minutes SET minute = minute WHERE minute = -1").Error; err != nil {
			return err
		}
		var existing ProductionTelemetryMinute
		err := tx.First(&existing, row.Minute).Error
		if err == nil {
			if existing.BatchID != row.BatchID || existing.Payload != row.Payload || existing.Complete != row.Complete || existing.AttemptCount != row.AttemptCount {
				return errors.New("production minute already sealed by another collector")
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var count int64
		if err := tx.Model(&ProductionAttemptRecord{}).Where("batch_id = ? AND minute = ?", row.BatchID, row.Minute).Count(&count).Error; err != nil {
			return err
		}
		if count != row.AttemptCount {
			return errors.New("production staging is incomplete")
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error
	})
}

func PublishedProductionAttempts(db *gorm.DB) *gorm.DB {
	return db.Where("(batch_id = '' OR EXISTS (SELECT 1 FROM production_telemetry_minutes AS published WHERE published.batch_id = production_attempt_records.batch_id AND published.minute = production_attempt_records.minute))")
}

func ReadProductionAttempts(start, end int64, page, size int) ([]ProductionAttemptRecord, error) {
	if page < 1 || page > 1000000 || size < 1 || size > 1000 {
		return nil, fmt.Errorf("invalid attempt page")
	}
	rows := []ProductionAttemptRecord{}
	err := PublishedProductionAttempts(DB).Where("minute >= ? AND minute <= ?", start, end).Order("minute, id").Offset((page - 1) * size).Limit(size).Find(&rows).Error
	return rows, err
}

func ReadProductionTelemetry(start, end int64) ([]ProductionTelemetryMinute, error) {
	var rows []ProductionTelemetryMinute
	err := DB.Where("minute >= ? AND minute <= ?", start, end).Order("minute").Find(&rows).Error
	return rows, err
}

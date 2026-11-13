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
	Minute    int64  `gorm:"primaryKey;autoIncrement:false" json:"minute"`
	BatchID   string `gorm:"size:64;not null" json:"batch_id"`
	Complete  bool   `gorm:"not null" json:"complete"`
	Payload   string `gorm:"type:text;not null" json:"-"`
	CreatedAt int64  `gorm:"autoCreateTime" json:"created_at"`
}

type ProductionAttemptRecord struct {
	Attempts      int    `json:"delivery_attempts"`
	DeliveredAt   int64  `gorm:"index" json:"delivered_at"`
	NextAttemptAt int64  `json:"next_attempt_at"`
	LastError     string `gorm:"size:160" json:"last_error"`
	ID            string `gorm:"primaryKey;size:64" json:"attempt_id"`
	Minute        int64  `gorm:"index" json:"minute"`
	Payload       string `gorm:"type:text;not null" json:"-"`
}

func SaveProductionTelemetry(rows []ProductionTelemetryMinute, now time.Time, attempts ...ProductionAttemptRecord) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&OptionRevision{}).Where("id = 1").UpdateColumn("version", gorm.Expr("version")).Error; err != nil {
			return err
		}
		for _, row := range rows {
			if row.Minute <= 0 || row.Minute%60 != 0 || row.BatchID == "" || row.Minute >= now.Unix()/60*60 || len(row.Payload) > 64<<20 {
				return errors.New("invalid production minute")
			}
			var existing ProductionTelemetryMinute
			err := tx.First(&existing, row.Minute).Error
			if err == nil {
				if existing.BatchID != row.BatchID || existing.Payload != row.Payload || existing.Complete != row.Complete {
					return errors.New("production minute already sealed by another collector")
				}
				continue
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
				return err
			}
		}
		if len(attempts) > 0 {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(attempts, 100).Error; err != nil {
				return err
			}
		}
		cutoff := now.Add(-24*time.Hour).Unix() / 60 * 60
		if err := tx.Where("minute < ?", cutoff).Delete(&ProductionAttemptRecord{}).Error; err != nil {
			return err
		}
		// Keep the retry journal bounded independently of aggregate retention.
		var boundary ProductionAttemptRecord
		if err := tx.Order("minute DESC, id DESC").Offset(99999).Limit(1).Find(&boundary).Error; err != nil {
			return err
		}
		if boundary.ID != "" {
			if err := tx.Where("minute < ? OR (minute = ? AND id < ?)", boundary.Minute, boundary.Minute, boundary.ID).Delete(&ProductionAttemptRecord{}).Error; err != nil {
				return err
			}
		}
		return tx.Where("minute < ?", cutoff).Delete(&ProductionTelemetryMinute{}).Error
	})
}

func ReadProductionAttempts(start, end int64, page, size int) ([]ProductionAttemptRecord, error) {
	if page < 1 || page > 1000000 || size < 1 || size > 1000 {
		return nil, fmt.Errorf("invalid attempt page")
	}
	rows := []ProductionAttemptRecord{}
	err := DB.Where("minute >= ? AND minute <= ?", start, end).Order("minute, id").Offset((page - 1) * size).Limit(size).Find(&rows).Error
	return rows, err
}

func ReadProductionTelemetry(start, end int64) ([]ProductionTelemetryMinute, error) {
	var rows []ProductionTelemetryMinute
	err := DB.Where("minute >= ? AND minute <= ?", start, end).Order("minute").Find(&rows).Error
	return rows, err
}

package model

import (
	"time"

	"gorm.io/gorm"
)

func (entry *MoneyEntry) BeforeCreate(tx *gorm.DB) error {
	if entry.BusinessDate != "" {
		return nil
	}
	at := time.Now()
	if entry.CreatedAt > 0 {
		at = time.Unix(entry.CreatedAt, 0)
	}
	day, err := captureBusinessDay(tx, at)
	if err != nil {
		return err
	}
	entry.BusinessDaySnapshot = day
	return nil
}

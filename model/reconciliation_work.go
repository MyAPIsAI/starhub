package model

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ReconciliationWork coalesces changes to a business day in the source transaction.
type ReconciliationWork struct {
	BusinessDaySnapshot
	ID         string `gorm:"primaryKey;size:64"`
	UserID     int64  `gorm:"index;not null"`
	Revision   int64  `gorm:"not null"`
	Dirty      bool   `gorm:"index:idx_reconciliation_pending,priority:1;not null"`
	EligibleAt int64  `gorm:"index:idx_reconciliation_pending,priority:2;not null"`
}

func queueReconciliationDay(tx *gorm.DB, userID int64, day BusinessDaySnapshot) error {
	if userID <= 0 {
		return nil
	}
	loc, err := time.LoadLocation(day.BusinessTimezone)
	if err != nil {
		return err
	}
	end := time.Unix(day.BusinessEnd, 0).In(loc)
	eligible := time.Date(end.Year(), end.Month(), end.Day(), 2, 0, 0, 0, loc)
	if eligible.Hour() < 2 {
		eligible = eligible.Add(time.Hour)
	}
	row := ReconciliationWork{ID: reportKey(userID, day), UserID: userID, BusinessDaySnapshot: day, Revision: 1, Dirty: true, EligibleAt: eligible.Unix()}
	return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoUpdates: clause.Assignments(map[string]any{"dirty": true, "revision": gorm.Expr("reconciliation_works.revision + 1")})}).Create(&row).Error
}

func (entry *Log) AfterCreate(tx *gorm.DB) error {
	day := entry.BusinessDaySnapshot
	if day.BusinessDate == "" {
		at := entry.AdmittedAt
		if at == 0 {
			at = entry.CreatedAt
		}
		day = legacyBusinessDay(at)
	}
	return queueReconciliationDay(tx, entry.UserID, day)
}

func PendingReconciliationWork(ctx context.Context, now time.Time) ([]ReconciliationWork, error) {
	rows := []ReconciliationWork{}
	err := DB.WithContext(ctx).Where("dirty = ? AND eligible_at <= ?", true, now.Unix()).Order("eligible_at, id").Limit(128).Find(&rows).Error
	return rows, err
}

func AckReconciliationWork(ctx context.Context, row ReconciliationWork) error {
	return DB.WithContext(ctx).Model(&ReconciliationWork{}).Where("id = ? AND revision = ?", row.ID, row.Revision).UpdateColumn("dirty", false).Error
}

func RetryReconciliationWork(ctx context.Context, row ReconciliationWork) error {
	return DB.WithContext(ctx).Model(&ReconciliationWork{}).Where("id = ? AND revision = ?", row.ID, row.Revision).UpdateColumn("eligible_at", time.Now().Add(5*time.Minute).Unix()).Error
}

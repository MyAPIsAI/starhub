package model

import (
	"context"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"starhub/config"
	"time"
)

type ReconciliationDirtyDay struct {
	UserID   int64  `gorm:"primaryKey;autoIncrement:false"`
	Day      string `gorm:"primaryKey"`
	Timezone string `gorm:"primaryKey"`
}

type ReconciliationSchedule struct {
	ID           int64 `gorm:"primaryKey;autoIncrement:false"`
	CompletedDay string
	Timezone     string
}

func normalizeBillingTime(entry *Log) {
	if entry.SettledAt == 0 {
		entry.SettledAt = time.Now().Unix()
	}
	if entry.AcceptedAt > 0 {
		entry.CreatedAt = entry.AcceptedAt
	}
	if entry.CreatedAt == 0 {
		entry.CreatedAt = entry.SettledAt
	}
}

func markReconciliationDirty(tx *gorm.DB, userID, at int64) error {
	zone := config.RegionTimezone()
	var profile DeploymentProfileVersion
	if err := tx.Order("revision DESC").Limit(1).Find(&profile).Error; err != nil {
		return err
	}
	if profile.Revision > 0 {
		zone = profile.Profile.Timezone
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return err
	}
	row := ReconciliationDirtyDay{UserID: userID, Day: time.Unix(at, 0).In(loc).Format("2006-01-02"), Timezone: loc.String()}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error
}

// RunScheduledReconciliation catches up missed 02:00 runs and late settlements.
// The cursor advances only after complete reports have been committed.
func RunScheduledReconciliation(ctx context.Context, now time.Time) error {
	profile, err := CurrentDeploymentProfile(ctx)
	if err != nil {
		return err
	}
	loc, err := time.LoadLocation(profile.Profile.Timezone)
	if err != nil {
		return err
	}
	local := now.In(loc)
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	due := midnight.AddDate(0, 0, -1)
	if local.Hour() < 2 {
		due = due.AddDate(0, 0, -1)
	}
	var state ReconciliationSchedule
	if err := DB.WithContext(ctx).Where("id = 1").Limit(1).Find(&state).Error; err != nil {
		return err
	}
	first := due
	if state.CompletedDay != "" && state.Timezone == loc.String() {
		previous, err := time.ParseInLocation("2006-01-02", state.CompletedDay, loc)
		if err != nil {
			return err
		}
		first = previous.AddDate(0, 0, 1)
	}
	for day := first; !day.After(due); day = day.AddDate(0, 0, 1) {
		if _, err := RunDailyReconciliation(ctx, day.Format("2006-01-02"), 0, 0, now); err != nil {
			return err
		}
		state = ReconciliationSchedule{ID: 1, CompletedDay: day.Format("2006-01-02"), Timezone: loc.String()}
		if err := DB.WithContext(ctx).Save(&state).Error; err != nil {
			return err
		}
	}
	var dirty []ReconciliationDirtyDay
	if err := DB.WithContext(ctx).Where("day <= ?", due.Format("2006-01-02")).Order("day,user_id").Find(&dirty).Error; err != nil {
		return err
	}
	for _, d := range dirty {
		if _, err := RunDailyReconciliation(ctx, d.Day, d.UserID, 0, now); err != nil {
			return err
		}
		if d.Timezone != loc.String() {
			if err := DB.WithContext(ctx).Delete(&d).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

package model

import (
	"errors"
	"gorm.io/gorm"
	"os"
	"time"
)

type BusinessDaySnapshot struct {
	BusinessCalendarVersion int    `gorm:"not null;default:0" json:"business_calendar_version"`
	BusinessDate            string `gorm:"size:10;not null;default:'';index" json:"business_date,omitempty"`
	BusinessTimezone        string `gorm:"size:64;not null;default:''" json:"business_timezone,omitempty"`
	BusinessRegion          string `gorm:"size:64;not null;default:''" json:"business_region,omitempty"`
	BusinessRegionRevision  int64  `gorm:"not null;default:0" json:"business_region_revision"`
	BusinessStart           int64  `gorm:"not null;default:0" json:"business_start"`
	BusinessEnd             int64  `gorm:"not null;default:0" json:"business_end"`
}

func BusinessDayForDate(date, timezone, region string, revision int64) (BusinessDaySnapshot, error) {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return BusinessDaySnapshot{}, err
	}
	day, err := time.ParseInLocation("2006-01-02", date, location)
	if err != nil {
		return BusinessDaySnapshot{}, err
	}
	return BusinessDaySnapshot{BusinessCalendarVersion: 1, BusinessDate: date, BusinessTimezone: timezone, BusinessRegion: region, BusinessRegionRevision: revision, BusinessStart: day.Unix(), BusinessEnd: day.AddDate(0, 0, 1).Unix()}, nil
}

func captureBusinessDay(tx *gorm.DB, at time.Time) (BusinessDaySnapshot, error) {
	var profile RegionProfile
	err := tx.First(&profile, "id = ?", ActiveRegionID()).Error
	if errors.Is(err, gorm.ErrRecordNotFound) && os.Getenv("STARHUB_REGION_PROFILE") == "" {
		return BusinessDayForDate(at.UTC().Format("2006-01-02"), "UTC", "", 0)
	}
	if err != nil {
		return BusinessDaySnapshot{}, ErrRegionUnavailable
	}
	location, err := time.LoadLocation(profile.Timezone)
	if err != nil {
		return BusinessDaySnapshot{}, ErrRegionUnavailable
	}
	return BusinessDayForDate(at.In(location).Format("2006-01-02"), profile.Timezone, profile.ID, profile.Revision)
}

// Old reservations retain the UTC contract they were accepted under. A later
// deployment timezone must never be used to reinterpret their accounting day.
func legacyBusinessDay(at int64) BusinessDaySnapshot {
	if at == 0 {
		at = time.Now().Unix()
	}
	day, _ := BusinessDayForDate(time.Unix(at, 0).UTC().Format("2006-01-02"), "UTC", "", 0)
	day.BusinessCalendarVersion = 0
	return day
}

func assignReservationDay(log *Log, reservation QuotaReservation) {
	log.BusinessDaySnapshot = reservation.BusinessDaySnapshot
	if log.BusinessDate == "" {
		log.BusinessDaySnapshot = legacyBusinessDay(reservation.AdmittedAt)
	}
}

package model

import (
	"errors"
	"time"
	_ "time/tzdata"

	"gorm.io/gorm"
	"starhub/config"
)

// TokenQuotaPeriod keeps settled usage after a daily or monthly rollover.
type TokenQuotaPeriod struct {
	TokenID   int64 `gorm:"primaryKey;autoIncrement:false" json:"token_id"`
	StartedAt int64 `gorm:"primaryKey;autoIncrement:false" json:"started_at"`
	EndedAt   int64 `json:"ended_at"`
	UsedQuota int64 `json:"used_quota"`
}

func tokenPeriodStart(period, zone string, now time.Time) int64 {
	location, err := time.LoadLocation(zone)
	if err != nil {
		location = time.UTC
	}
	local := now.In(location)
	day := local.Day()
	if period == "monthly" {
		day = 1
	}
	return time.Date(local.Year(), local.Month(), day, 0, 0, 0, 0, location).Unix()
}

func initializeTokenPeriod(token *Token) {
	if token.QuotaPeriod == "" {
		token.QuotaPeriod = "never"
	}
	if token.QuotaTimezone == "" {
		token.QuotaTimezone = config.RegionTimezone()
	}
	if token.QuotaLimit < 0 || (token.QuotaLimit == 0 && token.RemainQuota+token.UsedQuota > 0) {
		token.QuotaLimit = token.RemainQuota + token.UsedQuota
	}
	if token.PeriodStartedAt == 0 {
		token.PeriodStartedAt = time.Now().Unix()
		if token.QuotaPeriod != "never" {
			token.PeriodStartedAt = tokenPeriodStart(token.QuotaPeriod, token.QuotaTimezone, time.Now())
		}
	}
}

func validateTokenPeriod(period string, limit int64) error {
	if (period != "never" && period != "daily" && period != "monthly") || limit < 0 {
		return errors.New("invalid quota period or limit")
	}
	return nil
}

// RefreshTokenPeriod uses the same SQLite writer lock as quota reservation.
// Outstanding reservations remain held until their original requests settle.
func RefreshTokenPeriod(token *Token, now time.Time) error {
	if token.QuotaPeriod == "" || token.QuotaPeriod == "never" || token.PeriodStartedAt >= tokenPeriodStart(token.QuotaPeriod, token.QuotaTimezone, now) {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Token{}).Where("id = ?", token.ID).UpdateColumn("id", gorm.Expr("id")).Error; err != nil {
			return err
		}
		if err := tx.First(token, token.ID).Error; err != nil {
			return err
		}
		return refreshTokenPeriod(tx, token, now)
	})
}

func refreshTokenPeriod(tx *gorm.DB, token *Token, now time.Time) error {
	if token.QuotaPeriod == "" || token.QuotaPeriod == "never" {
		return nil
	}
	start := tokenPeriodStart(token.QuotaPeriod, token.QuotaTimezone, now)
	if token.PeriodStartedAt >= start {
		return nil
	}
	if token.PeriodStartedAt > 0 {
		history := TokenQuotaPeriod{TokenID: token.ID, StartedAt: token.PeriodStartedAt, EndedAt: start, UsedQuota: token.PeriodUsedQuota}
		if err := tx.Create(&history).Error; err != nil {
			return err
		}
	}
	reserved := max(int64(0), token.QuotaLimit-token.PeriodUsedQuota-token.RemainQuota)
	token.PeriodStartedAt, token.PeriodUsedQuota = start, 0
	if !token.UnlimitedQuota {
		token.RemainQuota = max(int64(0), token.QuotaLimit-reserved)
	}
	return tx.Model(&Token{}).Where("id = ?", token.ID).Updates(map[string]any{"period_started_at": start, "period_used_quota": 0, "remain_quota": token.RemainQuota}).Error
}

func prepareTokenPeriodUpdate(tx *gorm.DB, token *Token, fields map[string]any) error {
	period, limit := token.QuotaPeriod, token.QuotaLimit
	if period == "" {
		period = "never"
	}
	if value, ok := fields["quota_period"].(string); ok {
		period = value
	}
	if value, ok := fields["quota_limit"].(int64); ok {
		limit = value
	}
	if err := validateTokenPeriod(period, limit); err != nil {
		return err
	}
	_, limitChanged := fields["quota_limit"]
	periodChanged := period != token.QuotaPeriod
	if !periodChanged && !limitChanged {
		return nil
	}
	used := token.PeriodUsedQuota
	if token.QuotaPeriod == "never" || token.QuotaPeriod == "" {
		used = token.UsedQuota
	}
	reserved := int64(0)
	if !token.UnlimitedQuota {
		reserved = max(int64(0), token.QuotaLimit-used-token.RemainQuota)
	}
	if periodChanged {
		if token.PeriodStartedAt > 0 {
			history := TokenQuotaPeriod{TokenID: token.ID, StartedAt: token.PeriodStartedAt, EndedAt: time.Now().Unix(), UsedQuota: token.PeriodUsedQuota}
			if err := tx.Save(&history).Error; err != nil {
				return err
			}
		}
		used = 0
		if period == "never" {
			used = token.UsedQuota
		}
		fields["period_used_quota"] = used
		fields["period_started_at"] = max(time.Now().Unix(), token.PeriodStartedAt+1)
	}
	unlimited := token.UnlimitedQuota
	if value, ok := fields["unlimited_quota"].(bool); ok {
		unlimited = value
	}
	if !unlimited {
		if limit < used || limit-used < reserved {
			return errors.New("quota limit is below settled usage and in-flight reservations")
		}
		fields["remain_quota"] = limit - used - reserved
	}
	fields["quota_limit"] = limit
	return nil
}

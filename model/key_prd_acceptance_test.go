package model

import (
	"errors"
	"testing"
	"time"

	"starhub/common"
)

func TestKeyPRDAC07AC08AC09RegionalBoundariesAndLifetime(t *testing.T) {
	for _, period := range []string{"daily", "monthly"} {
		t.Run(period, func(t *testing.T) {
			newTestDB(t)
			user, key := newBillingUser(t, 100*common.QuotaPerUSD)
			loc, _ := time.LoadLocation("Asia/Dubai")
			now := time.Now().In(loc)
			start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
			boundary := start.AddDate(0, 0, 1)
			if period == "monthly" {
				start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
				boundary = start.AddDate(0, 1, 0)
			}
			if err := DB.Model(key).Updates(map[string]any{"quota_period": period, "quota_timezone": "Asia/Dubai", "quota_limit": int64(10 * common.QuotaPerUSD), "remain_quota": int64(0), "used_quota": int64(10 * common.QuotaPerUSD), "period_used_quota": int64(10 * common.QuotaPerUSD), "period_started_at": start.Unix()}).Error; err != nil {
				t.Fatal(err)
			}
			key, _ = GetTokenByID(key.ID)
			if err := RefreshTokenPeriod(key, boundary.Add(-time.Nanosecond)); err != nil {
				t.Fatal(err)
			}
			if key.RemainQuota != 0 || key.PeriodUsedQuota != 10*common.QuotaPerUSD {
				t.Error("reset before regional midnight")
			}
			if err := RefreshTokenPeriod(key, boundary); err != nil {
				t.Fatal(err)
			}
			if key.RemainQuota != 10*common.QuotaPerUSD || key.PeriodUsedQuota != 0 || key.UsedQuota != 10*common.QuotaPerUSD {
				t.Error("boundary lost period or lifetime usage")
			}
			for _, amount := range []int64{common.QuotaPerUSD, 2 * common.QuotaPerUSD} {
				if err := ReserveQuota(user.ID, key.ID, false, amount); err != nil {
					t.Fatal(err)
				}
				if err := SettleQuota(user.ID, key.ID, false, 0, amount, &Log{TokenPeriodStartedAt: key.PeriodStartedAt, Status: 1}); err != nil {
					t.Fatal(err)
				}
				key, _ = GetTokenByID(key.ID)
				if period == "daily" {
					boundary = boundary.AddDate(0, 0, 1)
				} else {
					boundary = boundary.AddDate(0, 1, 0)
				}
				if err := RefreshTokenPeriod(key, boundary); err != nil {
					t.Fatal(err)
				}
			}
			var history []TokenQuotaPeriod
			if err := DB.Where("token_id = ?", key.ID).Find(&history).Error; err != nil {
				t.Fatal(err)
			}
			var sum int64
			for _, entry := range history {
				sum += entry.UsedQuota
			}
			if len(history) != 3 || key.UsedQuota != 13*common.QuotaPerUSD || sum+key.PeriodUsedQuota != key.UsedQuota {
				t.Errorf("lifetime/history mismatch: periods=%d sum=%d lifetime=%d", len(history), sum, key.UsedQuota)
			}
		})
	}
}

func TestKeyPRD5ActualCostMayExceedKeyRemainder(t *testing.T) {
	newTestDB(t)
	user, key := newBillingUser(t, 1000000)
	if err := DB.Model(key).Updates(map[string]any{"quota_limit": int64(10000), "remain_quota": int64(10000)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := ReserveQuota(user.ID, key.ID, false, 9900); err != nil {
		t.Fatal(err)
	}
	entry := &Log{Status: 1}
	if err := SettleQuota(user.ID, key.ID, false, 200, 10100, entry); err != nil {
		t.Fatal(err)
	}
	key, _ = GetTokenByID(key.ID)
	user, _ = GetUserByID(user.ID)
	t.Logf("actual=10100 collected=%d unfunded=%d key_used=%d account_used=%d remaining=%d", entry.Quota, entry.UnfundedQuota, key.UsedQuota, user.UsedQuota, key.RemainQuota)
	if entry.Quota != 10100 || entry.UnfundedQuota != 0 || key.UsedQuota != 10100 || user.UsedQuota != 10100 || user.Quota != 989900 {
		t.Error("actual request cost was capped at the key limit despite sufficient account balance")
	}
	if key.RemainQuota != -100 {
		t.Errorf("over-limit remainder=%d, expected -100", key.RemainQuota)
	}
	if err := ReserveQuota(user.ID, key.ID, false, 1); !errors.Is(err, ErrTokenQuotaShort) {
		t.Errorf("over-limit key accepted another reservation: %v", err)
	}
}

func TestKeyOverrunClearsAtRegionalResetWithoutLosingHistory(t *testing.T) {
	newTestDB(t)
	user, key := newBillingUser(t, 1000000)
	loc, _ := time.LoadLocation("Asia/Dubai")
	now := time.Now().In(loc)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	if err := DB.Model(key).Updates(map[string]any{"quota_period": "daily", "quota_timezone": "Asia/Dubai", "quota_limit": int64(10000), "remain_quota": int64(10000), "period_started_at": start.Unix()}).Error; err != nil {
		t.Fatal(err)
	}
	if err := ReserveQuota(user.ID, key.ID, false, 9900); err != nil {
		t.Fatal(err)
	}
	if err := SettleQuota(user.ID, key.ID, false, 200, 10100, &Log{TokenPeriodStartedAt: start.Unix(), Status: 1}); err != nil {
		t.Fatal(err)
	}
	key, _ = GetTokenByID(key.ID)
	if key.RemainQuota != -100 || key.PeriodUsedQuota != 10100 {
		t.Fatal("actual cost did not exhaust this period")
	}
	if err := RefreshTokenPeriod(key, start.AddDate(0, 0, 1)); err != nil {
		t.Fatal(err)
	}
	var history TokenQuotaPeriod
	if err := DB.First(&history, "token_id = ? AND started_at = ?", key.ID, start.Unix()).Error; err != nil {
		t.Fatal(err)
	}
	if key.RemainQuota != 10000 || key.PeriodUsedQuota != 0 || key.UsedQuota != 10100 || history.UsedQuota != 10100 {
		t.Fatal("reset carried over spent quota or lost actual cost")
	}
	if err := ReserveQuota(user.ID, key.ID, false, 10000); err != nil {
		t.Fatalf("reset key cannot use its full new allowance: %v", err)
	}
}

package model

import (
	"errors"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"
)

const tokenUsageTime = "CASE WHEN admitted_at > 0 THEN admitted_at ELSE created_at END"

type TokenPeriodSummary struct {
	Period            string `json:"period"`
	Timezone          string `json:"timezone"`
	PeriodID          int64  `json:"period_id"`
	Start             int64  `json:"start"`
	End               int64  `json:"end"`
	UsedQuota         int64  `json:"used_quota"`
	ReservedQuota     int64  `json:"reserved_quota"`
	QuotaLimit        *int64 `json:"quota_limit"`
	RemainingQuota    *int64 `json:"remaining_quota"`
	LifetimeUsedQuota int64  `json:"lifetime_used_quota"`
}

type TokenUsageAmounts struct {
	Requests         int64 `json:"requests"`
	Quota            int64 `json:"quota"`
	UnfundedQuota    int64 `json:"unfunded_quota"`
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	SuccessCount     int64 `json:"success_count"`
	FailedCount      int64 `json:"failed_count"`
}

type TokenDayUsage struct {
	Date              string `json:"date"`
	TokenUsageAmounts `gorm:"embedded"`
}

type TokenModelUsage struct {
	Model             string `json:"model"`
	TokenUsageAmounts `gorm:"embedded"`
}

type TokenUsageDetails struct {
	Schema      string             `json:"schema"`
	UserID      int64              `json:"user_id"`
	TokenID     int64              `json:"token_id"`
	ObservedAt  int64              `json:"observed_at"`
	Current     TokenPeriodSummary `json:"current"`
	Days        int                `json:"days"`
	Trend       []TokenDayUsage    `json:"trend"`
	Models      []TokenModelUsage  `json:"models"`
	ModelPage   int                `json:"model_page"`
	ModelTotal  int64              `json:"model_total"`
	Periods     []TokenQuotaPeriod `json:"periods"`
	PeriodPage  int                `json:"period_page"`
	PeriodTotal int64              `json:"period_total"`
	PageSize    int                `json:"page_size"`
}

const tokenUsageSums = "COUNT(*) AS requests, COALESCE(SUM(quota),0) AS quota, COALESCE(SUM(unfunded_quota),0) AS unfunded_quota, COALESCE(SUM(prompt_tokens),0) AS prompt_tokens, COALESCE(SUM(completion_tokens),0) AS completion_tokens, COALESCE(SUM(CASE WHEN status = 1 THEN 1 ELSE 0 END),0) AS success_count, COALESCE(SUM(CASE WHEN status IN (2,3) THEN 1 ELSE 0 END),0) AS failed_count"

// ReadOwnedTokenUsage groups exact admission timestamps into regional calendar
// days. Missing days are explicit zeros; legacy logs use completion timestamps.
func ReadOwnedTokenUsage(owner, tokenID int64, periodPage, modelPage int, now time.Time) (*TokenUsageDetails, error) {
	if periodPage < 1 || periodPage > 1000000 || modelPage < 1 || modelPage > 1000000 {
		return nil, ErrTokenPeriodPolicy
	}
	var token Token
	if err := DB.First(&token, "id = ? AND user_id = ?", tokenID, owner).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTokenNotFound
		}
		return nil, err
	}
	if err := RefreshTokenQuota(&token, now); err != nil {
		return nil, err
	}
	view := &TokenUsageDetails{Schema: "token_usage_v1", UserID: owner, TokenID: tokenID, ObservedAt: now.Unix(), Days: 30, ModelPage: modelPage, PeriodPage: periodPage, PageSize: 20, Trend: []TokenDayUsage{}, Models: []TokenModelUsage{}, Periods: []TokenQuotaPeriod{}}
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&token, "id = ? AND user_id = ?", tokenID, owner).Error; err != nil {
			return ErrTokenNotFound
		}
		zoneName := token.QuotaTimezone
		if zoneName == "" {
			zoneName = "UTC"
		}
		zone, err := time.LoadLocation(zoneName)
		if err != nil {
			return err
		}
		current := TokenPeriodSummary{Period: token.QuotaPeriod, Timezone: zoneName, PeriodID: token.QuotaPeriodID, Start: token.PeriodStart, End: token.PeriodEnd, LifetimeUsedQuota: token.UsedQuota}
		if current.Period == "" {
			current.Period = "never"
		}
		limit, remaining := token.PeriodQuotaLimit, token.RemainQuota
		if tokenPeriodic(&token) {
			current.UsedQuota, current.ReservedQuota = token.PeriodUsedQuota, token.PeriodReservedQuota
		} else {
			current.UsedQuota = token.UsedQuota
			current.ReservedQuota, err = TokenPendingQuota(tx, tokenID)
			if err != nil {
				return err
			}
			if token.UsedQuota > math.MaxInt64-token.RemainQuota || current.ReservedQuota > math.MaxInt64-token.UsedQuota-token.RemainQuota {
				return ErrTokenPeriodPolicy
			}
			limit = token.UsedQuota + token.RemainQuota + current.ReservedQuota
		}
		if !token.UnlimitedQuota {
			current.QuotaLimit, current.RemainingQuota = &limit, &remaining
		}
		view.Current = current
		local := now.In(zone)
		today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, zone)
		start, end := today.AddDate(0, 0, -29), today.AddDate(0, 0, 1)
		query := func() *gorm.DB {
			return tx.Model(&Log{}).Where("user_id = ? AND token_id = ? AND ("+tokenUsageTime+") >= ? AND ("+tokenUsageTime+") < ?", owner, tokenID, start.Unix(), end.Unix())
		}
		var cases strings.Builder
		cases.WriteString("CASE")
		args := []any{}
		for i := 0; i < 30; i++ {
			day := start.AddDate(0, 0, i)
			date := day.Format("2006-01-02")
			cases.WriteString(" WHEN (" + tokenUsageTime + ") >= ? AND (" + tokenUsageTime + ") < ? THEN ?")
			args = append(args, day.Unix(), day.AddDate(0, 0, 1).Unix(), date)
			view.Trend = append(view.Trend, TokenDayUsage{Date: date})
		}
		cases.WriteString(" END AS date, " + tokenUsageSums)
		var daily []TokenDayUsage
		if err := query().Select(cases.String(), args...).Group("date").Scan(&daily).Error; err != nil {
			return err
		}
		byDate := map[string]TokenUsageAmounts{}
		for _, day := range daily {
			byDate[day.Date] = day.TokenUsageAmounts
		}
		for i := range view.Trend {
			view.Trend[i].TokenUsageAmounts = byDate[view.Trend[i].Date]
		}
		if err := query().Distinct("model").Count(&view.ModelTotal).Error; err != nil {
			return err
		}
		if err := query().Select("model, " + tokenUsageSums).Group("model").Order("quota DESC, model").Offset((modelPage - 1) * view.PageSize).Limit(view.PageSize).Scan(&view.Models).Error; err != nil {
			return err
		}
		periodQuery := func() *gorm.DB {
			return tx.Model(&TokenQuotaPeriod{}).Where("token_id = ? AND user_id = ?", tokenID, owner)
		}
		if err := periodQuery().Count(&view.PeriodTotal).Error; err != nil {
			return err
		}
		return periodQuery().Order("start DESC, id DESC").Offset((periodPage - 1) * view.PageSize).Limit(view.PageSize).Find(&view.Periods).Error
	})
	return view, err
}

package model

import (
	"errors"
	"math/big"
	"sort"
	"starhub/common"
	"strings"
	"time"

	"gorm.io/gorm"
)

const tokenUsageTime = "CASE WHEN admitted_at > 0 THEN admitted_at ELSE created_at END"

type TokenPeriodSummary struct {
	UsedQuotaNano         string  `json:"used_quota_nano"`
	ReservedQuotaNano     string  `json:"reserved_quota_nano"`
	QuotaLimitNano        *string `json:"quota_limit_nano"`
	RemainingQuotaNano    *string `json:"remaining_quota_nano"`
	LifetimeUsedQuotaNano string  `json:"lifetime_used_quota_nano"`
	Period                string  `json:"period"`
	Timezone              string  `json:"timezone"`
	PeriodID              int64   `json:"period_id"`
	Start                 int64   `json:"start"`
	End                   int64   `json:"end"`
	UsedQuota             int64   `json:"used_quota"`
	ReservedQuota         int64   `json:"reserved_quota"`
	QuotaLimit            *int64  `json:"quota_limit"`
	RemainingQuota        *int64  `json:"remaining_quota"`
	LifetimeUsedQuota     int64   `json:"lifetime_used_quota"`
}

type TokenUsageAmounts struct {
	QuotaNano         string `gorm:"-" json:"quota_nano"`
	UnfundedQuotaNano string `gorm:"-" json:"unfunded_quota_nano"`
	Requests          int64  `json:"requests"`
	Quota             int64  `json:"quota"`
	UnfundedQuota     int64  `json:"unfunded_quota"`
	PromptTokens      int64  `json:"prompt_tokens"`
	CompletionTokens  int64  `json:"completion_tokens"`
	SuccessCount      int64  `json:"success_count"`
	FailedCount       int64  `json:"failed_count"`
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

const tokenUsageSums = "COUNT(*) AS requests, COALESCE(SUM(prompt_tokens),0) AS prompt_tokens, COALESCE(SUM(completion_tokens),0) AS completion_tokens, COALESCE(SUM(CASE WHEN status = 1 THEN 1 ELSE 0 END),0) AS success_count, COALESCE(SUM(CASE WHEN status IN (2,3) THEN 1 ELSE 0 END),0) AS failed_count"

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

		used := common.QuotaMoney(token.UsedQuota, token.UsedQuotaRemainderNano)
		remainingMoney := common.QuotaMoney(token.RemainQuota, token.RemainQuotaRemainderNano)
		reservedMoney, err := TokenPendingMoney(tx, tokenID)
		if err != nil {
			return err
		}
		current.LifetimeUsedQuotaNano = used.String()
		limitMoney := new(big.Int).Add(new(big.Int).Set(used), remainingMoney)
		limitMoney.Add(limitMoney, reservedMoney)
		if tokenPeriodic(&token) {
			used = common.QuotaMoney(token.PeriodUsedQuota, token.PeriodUsedQuotaRemainderNano)
			reservedMoney = common.QuotaMoney(token.PeriodReservedQuota, token.PeriodReservedQuotaRemainderNano)
			limitMoney = common.QuotaMoney(token.PeriodQuotaLimit, token.PeriodQuotaLimitRemainderNano)
		}
		current.UsedQuota, current.ReservedQuota = legacyUsageQuota(used), legacyUsageQuota(reservedMoney)
		current.UsedQuotaNano, current.ReservedQuotaNano = used.String(), reservedMoney.String()
		if !token.UnlimitedQuota {
			limit, remaining := legacyUsageQuota(limitMoney), legacyUsageQuota(remainingMoney)
			limitNano, remainingNano := limitMoney.String(), remainingMoney.String()
			current.QuotaLimit, current.RemainingQuota = &limit, &remaining
			current.QuotaLimitNano, current.RemainingQuotaNano = &limitNano, &remainingNano
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
		cases.WriteString(" END")
		dayExpression := cases.String()
		var daily []TokenDayUsage
		if err := query().Select(dayExpression+" AS date, "+tokenUsageSums, args...).Group("date").Scan(&daily).Error; err != nil {
			return err
		}

		dayMoney, err := readGroupedLogMoney(query(), dayExpression, args...)
		if err != nil {
			return err
		}
		byDate := map[string]TokenUsageAmounts{}
		for _, day := range daily {
			byDate[day.Date] = day.TokenUsageAmounts
		}
		for i := range view.Trend {
			amount := byDate[view.Trend[i].Date]
			m := usageMoneyAt(dayMoney, view.Trend[i].Date)
			amount.Quota, amount.UnfundedQuota = legacyUsageQuota(&m.Charged), legacyUsageQuota(&m.Unfunded)
			amount.QuotaNano, amount.UnfundedQuotaNano = m.Charged.String(), m.Unfunded.String()
			view.Trend[i].TokenUsageAmounts = amount
		}
		var allModels []TokenModelUsage
		if err := query().Select("model, " + tokenUsageSums).Group("model").Scan(&allModels).Error; err != nil {
			return err
		}
		modelMoney, err := readGroupedLogMoney(query(), "model")
		if err != nil {
			return err
		}
		for i := range allModels {
			m := usageMoneyAt(modelMoney, allModels[i].Model)
			allModels[i].Quota, allModels[i].UnfundedQuota = legacyUsageQuota(&m.Charged), legacyUsageQuota(&m.Unfunded)
			allModels[i].QuotaNano, allModels[i].UnfundedQuotaNano = m.Charged.String(), m.Unfunded.String()
		}
		sort.Slice(allModels, func(i, j int) bool {
			left, right := usageMoneyAt(modelMoney, allModels[i].Model), usageMoneyAt(modelMoney, allModels[j].Model)
			if cmp := left.Charged.Cmp(&right.Charged); cmp != 0 {
				return cmp > 0
			}
			return allModels[i].Model < allModels[j].Model
		})
		view.ModelTotal = int64(len(allModels))
		offset := (modelPage - 1) * view.PageSize
		if offset < len(allModels) {
			view.Models = allModels[offset:min(offset+view.PageSize, len(allModels))]
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

package model

import (
	"errors"
	"math/big"
	"sort"
	"time"

	"starhub/common"
	"starhub/config"
)

func legacyUsageBuckets(owner, since int64, byModel bool) (map[string]*UsageStats, error) {
	loc, err := time.LoadLocation(config.RegionTimezone())
	if err != nil {
		return nil, err
	}
	buckets := map[string]*UsageStats{}
	visit := func(l Log) error {
		key := time.Unix(l.CreatedAt, 0).In(loc).Format("2006-01-02")
		if byModel {
			key += "\x00" + l.Model
		}
		if buckets[key] == nil {
			buckets[key] = &UsageStats{ID: key, Name: l.Model, AmountNano: "0"}
		}
		buckets[key].add(l)
		return nil
	}
	if owner > 0 {
		err = WalkLogs(DB, owner, LogFilters{Since: since}, visit)
	} else {
		// The legacy management summary preserves explicit all-account visibility.
		var last int64
		for {
			var rows []Log
			if err = DB.Where("created_at >= ? AND id > ?", since, last).Order("id ASC").Limit(500).Find(&rows).Error; err != nil {
				break
			}
			for _, l := range rows {
				_ = visit(l)
				last = l.ID
			}
			if len(rows) < 500 {
				break
			}
		}
	}
	return buckets, err
}
func legacyQuota(s *UsageStats) (int64, error) {
	q := new(big.Int).Quo(&s.amount, big.NewInt(1_000_000_000/common.QuotaPerUSD))
	if !q.IsInt64() {
		return 0, errors.New("legacy aggregate exceeds int64; use the exact usage report")
	}
	return q.Int64(), nil
}
func regionalDailyUsage(owner, since int64) ([]DailyUsage, error) {
	buckets, err := legacyUsageBuckets(owner, since, false)
	if err != nil {
		return nil, err
	}
	result := make([]DailyUsage, 0, len(buckets))
	for _, s := range buckets {
		q, e := legacyQuota(s)
		if e != nil {
			return nil, e
		}
		if !s.unfunded.IsInt64() {
			return nil, errors.New("unfunded aggregate exceeds int64")
		}
		result = append(result, DailyUsage{Date: s.ID, Requests: s.Requests, Quota: q, UnfundedQuota: s.unfunded.Int64(), PromptTokens: s.InputTokens, CompletionTokens: s.OutputTokens, SuccessCount: s.Successful, FailedCount: s.Requests - s.Successful})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Date < result[j].Date })
	return result, nil
}
func regionalModelUsage(owner, since int64) ([]ModelDailyUsage, error) {
	buckets, err := legacyUsageBuckets(owner, since, true)
	if err != nil {
		return nil, err
	}
	result := make([]ModelDailyUsage, 0, len(buckets))
	for _, s := range buckets {
		q, e := legacyQuota(s)
		if e != nil {
			return nil, e
		}
		result = append(result, ModelDailyUsage{Date: s.ID[:10], Model: s.Name, Requests: s.Requests, Quota: q, PromptTokens: s.InputTokens, CompletionTokens: s.OutputTokens})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Date == result[j].Date {
			return result[i].Model < result[j].Model
		}
		return result[i].Date < result[j].Date
	})
	return result, nil
}

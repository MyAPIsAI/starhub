package model

import (
	"context"
	"sort"
	"time"
)

type publicStatusCounts struct {
	Model                                                              string
	Date                                                               string
	Total, Success, DurationSum, DurationCount                         int64
	RecentTotal, RecentSuccess, RecentDurationSum, RecentDurationCount int64
}

const publicStatusCountSQL = `COUNT(*) AS total, SUM(CASE WHEN status = 1 THEN 1 ELSE 0 END) AS success,
	SUM(CASE WHEN duration_ms > 0 THEN duration_ms ELSE 0 END) AS duration_sum,
	SUM(CASE WHEN duration_ms > 0 THEN 1 ELSE 0 END) AS duration_count`

func statusMetric(total, success, durationSum, durationCount int64) PublicStatusMetric {
	metric := PublicStatusMetric{TotalRequests: total, SuccessCount: success, FailedCount: total - success}
	if total > 0 {
		rate := float64(success) / float64(total)
		metric.SuccessRate = &rate
	}
	if durationCount > 0 {
		average := durationSum / durationCount
		metric.AvgDurationMS = &average
	}
	return metric
}

func populatePublicStatusMetrics(ctx context.Context, view *PublicStatusOverview, scope []publicStatusScope) error {
	start, err := time.Parse("2006-01-02", view.HistoryStart)
	if err != nil {
		return err
	}
	if len(scope) == 0 {
		return nil
	}
	// Filter channel/model pairs without joining logs, so multi-group grants never multiply samples.
	pairs := map[int64]map[string]bool{}
	for _, row := range scope {
		if pairs[row.ChannelID] == nil {
			pairs[row.ChannelID] = map[string]bool{}
		}
		pairs[row.ChannelID][row.Model] = true
	}
	ids := make([]int64, 0, len(pairs))
	for id := range pairs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	filter := DB.Where("1 = 0")
	for _, id := range ids {
		names := make([]string, 0, len(pairs[id]))
		for name := range pairs[id] {
			names = append(names, name)
		}
		sort.Strings(names)
		filter = filter.Or("channel_id = ? AND model IN ?", id, names)
	}
	base := DB.WithContext(ctx).Model(&Log{}).Where("created_at >= ? AND created_at < ? AND status IN ?", start.Unix(), view.WindowEnd, []int{1, 2}).Where(filter)
	var totals []publicStatusCounts
	err = base.Select("model, "+publicStatusCountSQL+`,
		SUM(CASE WHEN created_at >= ? THEN 1 ELSE 0 END) AS recent_total,
		SUM(CASE WHEN created_at >= ? AND status = 1 THEN 1 ELSE 0 END) AS recent_success,
		SUM(CASE WHEN created_at >= ? AND duration_ms > 0 THEN duration_ms ELSE 0 END) AS recent_duration_sum,
		SUM(CASE WHEN created_at >= ? AND duration_ms > 0 THEN 1 ELSE 0 END) AS recent_duration_count`, view.WindowStart, view.WindowStart, view.WindowStart, view.WindowStart).
		Group("model").Scan(&totals).Error
	if err != nil {
		return err
	}
	var days []publicStatusCounts
	day := DateFromUnix("created_at")
	if err := base.Select("model, " + day + " AS date, " + publicStatusCountSQL).Group("model, " + day).Scan(&days).Error; err != nil {
		return err
	}
	byModel := map[string]publicStatusCounts{}
	byDay := map[string]map[string]PublicStatusMetric{}
	for _, row := range totals {
		byModel[row.Model] = row
	}
	for _, row := range days {
		if byDay[row.Model] == nil {
			byDay[row.Model] = map[string]PublicStatusMetric{}
		}
		byDay[row.Model][row.Date] = statusMetric(row.Total, row.Success, row.DurationSum, row.DurationCount)
	}
	unknown, affected, unavailable := false, false, 0
	for i := range view.Models {
		m := &view.Models[i]
		row := byModel[m.ID]
		m.Recent = statusMetric(row.RecentTotal, row.RecentSuccess, row.RecentDurationSum, row.RecentDurationCount)
		m.History = statusMetric(row.Total, row.Success, row.DurationSum, row.DurationCount)
		for date := start; date.Unix() < view.WindowEnd; date = date.AddDate(0, 0, 1) {
			label := date.Format("2006-01-02")
			m.Days = append(m.Days, PublicStatusDay{Date: label, PublicStatusMetric: byDay[m.ID][label]})
		}
		switch {
		case !m.Available:
			m.State = "unavailable"
			affected = true
			unavailable++
		case m.Recent.TotalRequests == 0:
			m.State = "unknown"
			unknown = true
		case m.Recent.FailedCount > 0:
			m.State = "degraded"
			affected = true
		default:
			m.State = "operational"
		}
	}
	switch {
	case unavailable == len(view.Models):
		view.State = "unavailable"
	case affected:
		view.State = "degraded"
	case unknown:
		view.State = "unknown"
	default:
		view.State = "operational"
	}
	return nil
}

package model

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

var ErrInvalidStatusPageMode = errors.New("status_page_mode must be auto, include or exclude")

// Classification follows M1 4.1. Customer errors, credentials, configuration
// errors, cancellations and unclassified historical failures are excluded.
var statusFailureCodes = []string{"upstream_unavailable", "tier_capacity_unavailable", "upstream_timeout", "bad_response", "stream_interrupted", "unknown_terminal", "retry_budget_exhausted", "CONNECT_FAIL", "FIRST_TOKEN_TIMEOUT", "5xx_RETRYABLE", "RATE_LIMITED", "BAD_RESPONSE", "STREAM_INTERRUPTED", "UNKNOWN_TERMINAL"}

// Match the public catalog's legacy ratio semantics without inventing metadata
// or resurrecting disabled, custom or explicitly deleted definitions.
func publicPageGroups(catalog *ServiceGroupCatalog) []ServiceGroupConfig {
	groups := append([]ServiceGroupConfig{}, catalog.Groups...)
	seen := map[string]bool{}
	for _, g := range groups {
		seen[g.GroupID] = true
	}
	for _, g := range catalog.deletedGroups {
		seen[g.GroupID] = true
	}
	for id := range catalog.GroupRatios {
		if !seen[id] {
			groups = append(groups, ServiceGroupConfig{GroupID: id, Name: id, Enabled: true, Visibility: "common", StatusPageMode: "auto"})
		}
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].GroupID < groups[j].GroupID })
	return groups
}

func PublicCommonGroups(catalog *ServiceGroupCatalog) []ServiceGroupConfig {
	groups := []ServiceGroupConfig{}
	for _, g := range publicPageGroups(catalog) {
		if _, ok := catalog.GroupRatios[g.GroupID]; ok && g.Enabled && g.Visibility == "common" {
			groups = append(groups, g)
		}
	}
	return groups
}

// Explicit per-group choices override the default highest two common groups.
// Disabled groups never participate. Private group names are not public metadata.
func StatusPageGroups(catalog *ServiceGroupCatalog) []ServiceGroupConfig {
	defaults := PublicCommonGroups(catalog)
	sort.Slice(defaults, func(i, j int) bool {
		a, b := catalog.GroupRatios[defaults[i].GroupID], catalog.GroupRatios[defaults[j].GroupID]
		if a != b {
			return a > b
		}
		return defaults[i].GroupID < defaults[j].GroupID
	})
	selected := map[string]bool{}
	for _, g := range defaults[:min(2, len(defaults))] {
		selected[g.GroupID] = true
	}
	result := []ServiceGroupConfig{}
	for _, g := range publicPageGroups(catalog) {
		if _, ok := catalog.GroupRatios[g.GroupID]; !ok || !g.Enabled {
			continue
		}
		included := selected[g.GroupID]
		if g.StatusPageMode == "include" {
			included = true
		}
		if g.StatusPageMode == "exclude" {
			included = false
		}
		if included {
			result = append(result, g)
		}
	}
	return result
}

type StatusWindow struct {
	Samples     int64    `json:"samples"`
	SuccessRate *float64 `json:"success_rate"`
	TTFTSamples int64    `json:"ttft_samples"`
	TTFTP50MS   *int64   `json:"ttft_p50_ms"`
	TTFTP95MS   *int64   `json:"ttft_p95_ms"`
}
type StatusDay struct {
	Date        string   `json:"date"`
	Samples     int64    `json:"samples"`
	Successful  int64    `json:"successful"`
	SuccessRate *float64 `json:"success_rate"`
	State       string   `json:"state"`
}
type PublicModelStatus struct {
	Current     CatalogStatus `json:"current"`
	Last24Hours StatusWindow  `json:"last_24_hours"`
	Daily       []StatusDay   `json:"daily"`
}
type statusCount struct {
	Model, Date         string
	Samples, Successful int64
}
type statusLatency struct {
	Model       string
	TTFT, Count int64
}

func statusRate(successful, samples int64) *float64 {
	if samples == 0 {
		return nil
	}
	value := float64(successful) / float64(samples)
	return &value
}
func statusBase(db *gorm.DB, names, groups []string, start, end int64) *gorm.DB {
	return db.Model(&Log{}).Where("model IN ? AND service_group IN ? AND created_at >= ? AND created_at <= ?", names, groups, start, end).
		Where("status = 1 OR (status = 2 AND error_code IN ?)", statusFailureCodes)
}
func statusCounts(db *gorm.DB, names, groups []string, start, end int64) ([]statusCount, error) {
	rows := []statusCount{}
	err := statusBase(db, names, groups, start, end).Select("model, COUNT(*) AS samples, SUM(CASE WHEN status=1 THEN 1 ELSE 0 END) AS successful").Group("model").Scan(&rows).Error
	return rows, err
}
func statusLatencies(db *gorm.DB, names, groups []string, start, end int64) (map[string]StatusWindow, error) {
	rows := []statusLatency{}
	err := db.Model(&Log{}).Where("model IN ? AND service_group IN ? AND created_at >= ? AND created_at <= ? AND status=1 AND prompt_tokens <= 2000 AND ttft_ms > 0 AND call_type IN ?", names, groups, start, end, []string{"", "chat", "responses"}).
		Select("model, ttft_ms AS ttft, COUNT(*) AS count").Group("model, ttft_ms").Order("model, ttft_ms").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	result := map[string]StatusWindow{}
	for _, row := range rows {
		w := result[row.Model]
		w.TTFTSamples += row.Count
		result[row.Model] = w
	}
	cumulative := map[string]int64{}
	for _, row := range rows {
		w := result[row.Model]
		cumulative[row.Model] += row.Count
		if w.TTFTP50MS == nil && cumulative[row.Model] >= int64(math.Ceil(float64(w.TTFTSamples)*0.5)) {
			v := row.TTFT
			w.TTFTP50MS = &v
		}
		if w.TTFTP95MS == nil && cumulative[row.Model] >= int64(math.Ceil(float64(w.TTFTSamples)*0.95)) {
			v := row.TTFT
			w.TTFTP95MS = &v
		}
		result[row.Model] = w
	}
	return result, nil
}

// Queries aggregate only requested published models. Calendar-day ranges are
// computed in the deployment timezone, including DST and partial current days.
// The daily CTE permits indexed range scans without returning raw call records.
func PublicStatusHistory(ctx context.Context, models map[string]bool, groups []ServiceGroupConfig, timezone string, at time.Time) (map[string]PublicModelStatus, error) {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, err
	}
	local := at.In(location)
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
	days := make([]StatusDay, 90)
	values := make([]string, 90)
	args := []any{}
	for i := range days {
		start := midnight.AddDate(0, 0, i-89)
		end := start.AddDate(0, 0, 1)
		days[i] = StatusDay{Date: start.Format("2006-01-02"), State: "insufficient_data"}
		values[i] = "(?,?,?)"
		args = append(args, days[i].Date, start.Unix(), end.Unix())
	}
	names, textNames, ids := []string{}, []string{}, []string{}
	var limit int64
	result := map[string]PublicModelStatus{}
	for name, text := range models {
		names = append(names, name)
		if text {
			textNames = append(textNames, name)
		}
		result[name] = PublicModelStatus{Current: CatalogStatus{State: "insufficient_data", WindowMinutes: 15, At: at.Unix()}, Daily: append([]StatusDay{}, days...)}
	}
	for _, g := range groups {
		ids = append(ids, g.GroupID)
		if g.SLAP95MS != nil && *g.SLAP95MS > limit {
			limit = *g.SLAP95MS
		}
	}
	if len(names) == 0 || len(ids) == 0 {
		return result, nil
	}
	err = DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		current, err := statusCounts(tx, names, ids, at.Add(-15*time.Minute).Unix(), at.Unix())
		if err != nil {
			return err
		}
		lastDay, err := statusCounts(tx, names, ids, at.Add(-24*time.Hour).Unix(), at.Unix())
		if err != nil {
			return err
		}
		shortLatency, err := statusLatencies(tx, textNames, ids, at.Add(-15*time.Minute).Unix(), at.Unix())
		if err != nil {
			return err
		}
		dayLatency, err := statusLatencies(tx, textNames, ids, at.Add(-24*time.Hour).Unix(), at.Unix())
		if err != nil {
			return err
		}
		for _, row := range current {
			entry := result[row.Model]
			entry.Current = summarizePublicStatus(row, shortLatency[row.Model], limit, at)
			result[row.Model] = entry
		}
		for _, row := range lastDay {
			entry := result[row.Model]
			entry.Last24Hours = dayLatency[row.Model]
			entry.Last24Hours.Samples = row.Samples
			entry.Last24Hours.SuccessRate = statusRate(row.Successful, row.Samples)
			result[row.Model] = entry
		}
		query := `WITH day_ranges(date, start_at, end_at) AS (VALUES ` + strings.Join(values, ",") + `)
 SELECT logs.model, d.date, COUNT(*) AS samples, SUM(CASE WHEN logs.status=1 THEN 1 ELSE 0 END) AS successful
 FROM day_ranges d JOIN logs ON logs.created_at >= d.start_at AND logs.created_at < d.end_at
 WHERE logs.created_at <= ? AND logs.model IN ? AND logs.service_group IN ? AND (logs.status=1 OR (logs.status=2 AND logs.error_code IN ?)) GROUP BY logs.model, d.date`
		args = append(args, at.Unix(), names, ids, statusFailureCodes)
		rows := []statusCount{}
		if err := tx.Raw(query, args...).Scan(&rows).Error; err != nil {
			return err
		}
		indexes := map[string]int{}
		for i, d := range days {
			indexes[d.Date] = i
		}
		for _, row := range rows {
			entry := result[row.Model]
			day := &entry.Daily[indexes[row.Date]]
			day.Samples = row.Samples
			day.Successful = row.Successful
			if row.Samples >= 50 {
				day.SuccessRate = statusRate(row.Successful, row.Samples)
				day.State = "normal"
				if *day.SuccessRate < 0.99 {
					day.State = "outage"
				} else if *day.SuccessRate < 0.999 {
					day.State = "degraded"
				}
			}
			result[row.Model] = entry
		}
		return nil
	})
	return result, err
}

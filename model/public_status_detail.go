package model

import (
	"context"
	"errors"
	"strings"
	"time"
)

var ErrInvalidStatusDate = errors.New("date must be within the last 90 calendar days")

// Filtering runs over the complete matching catalog before pagination, using
// only the short decision window rather than loading ninety days for every row.
func PublicStatusCurrent(ctx context.Context, models map[string]bool, groups []ServiceGroupConfig, at time.Time) (map[string]CatalogStatus, error) {
	result := map[string]CatalogStatus{}
	names, textNames, ids := []string{}, []string{}, []string{}
	var limit int64
	for name, text := range models {
		names = append(names, name)
		if text {
			textNames = append(textNames, name)
		}
		result[name] = CatalogStatus{State: "insufficient_data", WindowMinutes: 15, At: at.Unix()}
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
	rows, err := statusCounts(DB.WithContext(ctx), names, ids, at.Add(-15*time.Minute).Unix(), at.Unix())
	if err != nil {
		return nil, err
	}
	latency, err := statusLatencies(DB.WithContext(ctx), textNames, ids, at.Add(-15*time.Minute).Unix(), at.Unix())
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.Model] = summarizePublicStatus(row, latency[row.Model], limit, at)
	}
	return result, nil
}

func summarizePublicStatus(row statusCount, latency StatusWindow, limit int64, at time.Time) CatalogStatus {
	result := CatalogStatus{State: "insufficient_data", WindowMinutes: 15, At: at.Unix(), Samples: row.Samples}
	if row.Samples < 50 {
		return result
	}
	result.SuccessRate = statusRate(row.Successful, row.Samples)
	result.State = "normal"
	if *result.SuccessRate < 0.95 {
		result.State = "outage"
	} else if *result.SuccessRate < 0.99 {
		result.State = "degraded"
	}
	if latency.TTFTSamples >= 50 {
		result.TTFTP95MS = latency.TTFTP95MS
		if limit > 0 && latency.TTFTP95MS != nil && *latency.TTFTP95MS > limit && result.State == "normal" {
			result.State = "degraded"
		}
	}
	return result
}

type StatusHour struct {
	Start       int64    `json:"start"`
	End         int64    `json:"end"`
	Samples     int64    `json:"samples"`
	Successful  int64    `json:"successful"`
	SuccessRate *float64 `json:"success_rate"`
}
type PublicHourlyStatus struct {
	Start       int64        `json:"start"`
	End         int64        `json:"end"`
	Samples     int64        `json:"samples"`
	SuccessRate *float64     `json:"success_rate"`
	Hours       []StatusHour `json:"hours"`
}

// Empty date means the trailing 24 hours. A calendar date uses the deployment
// timezone and can contain 23 or 25 hours across DST. Future hours stay empty.
func PublicStatusHours(ctx context.Context, name, timezone, date string, groups []ServiceGroupConfig, at time.Time) (PublicHourlyStatus, error) {
	result := PublicHourlyStatus{Hours: []StatusHour{}}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return result, err
	}
	start, end := at.Add(-24*time.Hour), at
	if date != "" {
		start, err = time.ParseInLocation("2006-01-02", date, location)
		local := at.In(location)
		today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
		if err != nil || start.Before(today.AddDate(0, 0, -89)) || start.After(today) {
			return result, ErrInvalidStatusDate
		}
		end = start.AddDate(0, 0, 1)
	}
	result.Start, result.End = start.Unix(), end.Unix()
	values, args := []string{}, []any{}
	for cursor := start; cursor.Before(end); cursor = cursor.Add(time.Hour) {
		next := cursor.Add(time.Hour)
		if next.After(end) {
			next = end
		}
		index := len(result.Hours)
		result.Hours = append(result.Hours, StatusHour{Start: cursor.Unix(), End: next.Unix()})
		values = append(values, "(?,?,?)")
		upper := next.Unix()
		if date == "" && next.Equal(end) {
			upper++
		}
		args = append(args, index, cursor.Unix(), upper)
	}
	ids := []string{}
	for _, g := range groups {
		ids = append(ids, g.GroupID)
	}
	if len(ids) == 0 {
		return result, nil
	}
	args = append(args, at.Unix(), name, ids, statusFailureCodes)
	var rows []struct {
		Bucket              int
		Samples, Successful int64
	}
	err = DB.WithContext(ctx).Raw(`WITH hours(bucket,start_at,end_at) AS (VALUES `+strings.Join(values, ",")+`)
 SELECT h.bucket, COUNT(*) AS samples, SUM(CASE WHEN logs.status=1 THEN 1 ELSE 0 END) AS successful
 FROM hours h JOIN logs ON logs.created_at>=h.start_at AND logs.created_at<h.end_at
 WHERE logs.created_at<=? AND logs.model=? AND logs.service_group IN ? AND (logs.status=1 OR (logs.status=2 AND logs.error_code IN ?)) GROUP BY h.bucket`, args...).Scan(&rows).Error
	if err != nil {
		return result, err
	}
	var success int64
	for _, row := range rows {
		h := &result.Hours[row.Bucket]
		h.Samples = row.Samples
		h.Successful = row.Successful
		h.SuccessRate = statusRate(row.Successful, row.Samples)
		result.Samples += row.Samples
		success += row.Successful
	}
	result.SuccessRate = statusRate(success, result.Samples)
	return result, nil
}

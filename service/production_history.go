package service

import (
	"encoding/json"
	"starhub/metric"
	"time"
)

// ProductionWindowCoverage distinguishes a complete quarter from a partial
// requested range, an open minute, or collection loss. Empty traffic is valid
// only when every minute in the quarter was durably published as complete.
type ProductionWindowCoverage struct {
	Minute          int64 `json:"minute"`
	Expected        int   `json:"expected_minutes"`
	Requested       int   `json:"requested_minutes"`
	CompleteMinutes int   `json:"complete_minutes"`
	Complete        bool  `json:"complete"`
}

func ProductionQuarterMetrics(byModel bool, start, end int64, now time.Time) ([]metric.PlatformMetric, []ProductionWindowCoverage, error) {
	buckets, err := ProductionMetricBuckets(start, end, now)
	if err != nil {
		return nil, nil, err
	}
	rows := []metric.PlatformMetric{}
	coverage := []ProductionWindowCoverage{}
	for _, bucket := range buckets {
		at := bucket["minute"].(int64) / 900 * 900
		if len(coverage) == 0 || coverage[len(coverage)-1].Minute != at {
			coverage = append(coverage, ProductionWindowCoverage{Minute: at, Expected: 15})
		}
		window := &coverage[len(coverage)-1]
		window.Requested++
		for _, item := range bucket["models"].([]map[string]any) {
			if item["model_name"] == metric.CoverageModel {
				if item["coverage_status"] == "complete" {
					window.CompleteMinutes++
				}
				continue
			}
			raw, err := json.Marshal(item)
			if err != nil {
				return nil, nil, err
			}
			var row metric.PlatformMetric
			if err := json.Unmarshal(raw, &row); err != nil {
				return nil, nil, err
			}
			rows = append(rows, row)
		}
		window.Complete = window.CompleteMinutes == window.Expected
	}
	return metric.AggregatePlatformMetrics(rows, byModel, true), coverage, nil
}

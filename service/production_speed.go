package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"

	"starhub/metric"
	"starhub/model"
)

var productionTTFTBounds = [...]int64{10, 25, 50, 100, 250, 500, 1000, 2000, 5000, 10000, 30000, 60000, 120000, 300000}

// Hydration creates a derived projection only. Sealed manifests are never rewritten.
func hydrateProductionSpeed(ctx context.Context, b *metric.ProductionMinute, row model.ProductionTelemetryMinute, inputCap int) error {
	var records []model.ProductionAttemptRecord
	if err := model.PublishedProductionAttempts(model.DB.WithContext(ctx)).Where("minute = ? AND batch_id = ?", row.Minute, row.BatchID).Limit(100001).Find(&records).Error; err != nil {
		return err
	}
	if int64(len(records)) != row.AttemptCount {
		b.Complete, b.Error = false, "attempt_retention_gap"
		return nil
	}
	b.Attempts = make([]metric.ProductionAttempt, 0, len(records))
	for _, record := range records {
		var attempt metric.ProductionAttempt
		if err := json.Unmarshal([]byte(record.Payload), &attempt); err != nil || attempt.ID != record.ID {
			return fmt.Errorf("corrupt production speed journal")
		}
		b.Attempts = append(b.Attempts, attempt)
	}
	sort.Slice(b.Attempts, func(i, j int) bool {
		a, c := b.Attempts[i], b.Attempts[j]
		if a.EndedAt != c.EndedAt {
			return a.EndedAt < c.EndedAt
		}
		return a.ID < c.ID
	})
	for i := range b.Entries {
		entry := &b.Entries[i]
		if entry.Health == nil {
			continue
		}
		h := *entry.Health
		h.InputCap, h.TTFTCount, h.TTFTSum, h.Histogram = inputCap, 0, 0, [14]int64{}
		entry.RecentTTFT = nil
		for _, a := range b.Attempts {
			if !eligibleProductionSpeed(a, *entry, inputCap) {
				continue
			}
			h.TTFTCount++
			h.TTFTSum += a.FirstContentMS
			entry.RecentTTFT = append(entry.RecentTTFT, a.FirstContentMS)
			if len(entry.RecentTTFT) > 5 {
				entry.RecentTTFT = entry.RecentTTFT[1:]
			}
			for j, bound := range productionTTFTBounds {
				if a.FirstContentMS <= bound {
					h.Histogram[j]++
				}
			}
		}
		entry.Health = &h
	}
	return nil
}

func eligibleProductionSpeed(a metric.ProductionAttempt, e metric.ProductionEntry, cap int) bool {
	return a.ChannelID == e.ChannelID && a.Model == e.ModelName && a.Generation == e.Generation && a.Outcome == "success" && a.Stream && a.InputTokens > 0 && a.InputTokens <= int64(cap) && a.FirstContentMS > 0
}

func singleSpeedPart(minute int64, entry metric.ProductionEntry, attempts []metric.ProductionAttempt) map[string]any {
	cap := 2000
	if entry.Health != nil {
		cap = entry.Health.InputCap
	}
	var raw [98]int64
	var latest []map[string]any
	count := int64(0)
	for _, a := range attempts {
		if !eligibleProductionSpeed(a, entry, cap) || a.EndedAt < minute*1000 || a.EndedAt >= (minute+60)*1000 {
			continue
		}
		count++
		cell := min(97, max(1, 1+int(math.Ceil(math.Log2(math.Max(float64(a.FirstContentMS), .125)/.125)*2))))
		raw[cell]++
		latest = append(latest, map[string]any{"ended_at": strconv.FormatInt(a.EndedAt*1000, 10), "node_id": os.Getenv("NODE_NAME"), "attempt_id": a.ID, "raw_ms": a.FirstContentMS, "normalized_ms": a.FirstContentMS})
		if len(latest) > 5 {
			latest = latest[1:]
		}
	}
	var buckets [7]map[string]any
	for i := range buckets {
		buckets[i] = map[string]any{"count": int64(0), "raw_histogram": [98]int64{}, "normalized_histogram": [98]int64{}}
	}
	buckets[0] = map[string]any{"count": count, "raw_histogram": raw, "normalized_histogram": raw, "latest": latest}
	return map[string]any{"schema": 6, "contract": "single-input-ewma5-v1", "endpoint_generation": entry.Generation, "minute": minute, "input_cap": cap, "buckets": buckets}
}

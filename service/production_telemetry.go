package service

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"starhub/metric"
	"starhub/model"
)

var productionFlushMu sync.Mutex

func FlushProductionTelemetry(now time.Time) error {
	productionFlushMu.Lock()
	defer productionFlushMu.Unlock()
	batch := metric.ClosedProductionMinutes(now)
	if len(batch) == 0 {
		return nil
	}
	rows := make([]model.ProductionTelemetryMinute, 0, len(batch))
	attempts := []model.ProductionAttemptRecord{}
	for _, b := range batch {
		for _, a := range b.Attempts {
			raw, err := json.Marshal(a)
			if err != nil {
				return err
			}
			attempts = append(attempts, model.ProductionAttemptRecord{ID: a.ID, Minute: b.Minute, Payload: string(raw)})
		}
		count := int64(len(b.Attempts))
		b.Attempts = nil
		raw, err := json.Marshal(b)
		if err != nil {
			return err
		}
		rows = append(rows, model.ProductionTelemetryMinute{Minute: b.Minute, BatchID: b.BatchID, Complete: b.Complete, Payload: string(raw), AttemptCount: count})
	}
	if err := model.SaveProductionTelemetry(rows, now, attempts...); err != nil {
		return err
	}
	metric.AckProduction(batch)
	wakeProductionDelivery()
	return nil
}

func RunProductionTelemetry(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if err := FlushProductionTelemetry(now); err != nil {
				log.Printf("production telemetry flush failed: %v", err)
			}
		}
	}
}

func encodeProductionParts(parts []map[string]any) (string, error) {
	if len(parts) > 16 {
		return "", errors.New("too many endpoint generations")
	}
	raw, err := json.Marshal(parts)
	if err != nil {
		return "", err
	}
	if len(raw) > 1<<20 {
		return "", errors.New("production manifest exceeds decoded budget")
	}
	if len(raw) <= 60000 {
		return string(raw), nil
	}
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	if _, err = w.Write(raw); err != nil {
		return "", err
	}
	if err = w.Close(); err != nil {
		return "", err
	}
	encoded := "speed-z1:" + base64.StdEncoding.EncodeToString(buf.Bytes())
	if len(encoded) > 60000 {
		return "", errors.New("production manifest exceeds wire budget")
	}
	return encoded, nil
}

func productionPart(minute int64, generation string) map[string]any {
	return map[string]any{"schema": 5, "contract": metric.ProductionContract, "minute": minute, "endpoint_generation": generation}
}

// ProductionMetricBuckets emits every requested minute, including explicit
// missing and open markers. Only committed sealed data can assert completeness.
func ProductionMetricBuckets(start, end int64, now time.Time) ([]map[string]any, error) {
	start = max(start/60*60, now.Add(-24*time.Hour).Unix()/60*60)
	end = min(end/60*60, now.Unix()/60*60)
	stored, err := model.ReadProductionTelemetry(start, end)
	if err != nil {
		return nil, err
	}
	byMinute := map[int64]metric.ProductionMinute{}
	for _, row := range stored {
		var b metric.ProductionMinute
		if json.Unmarshal([]byte(row.Payload), &b) != nil || b.Minute != row.Minute || b.BatchID != row.BatchID || b.Complete != row.Complete {
			return nil, errors.New("corrupt production minute")
		}
		byMinute[row.Minute] = b
	}
	result := []map[string]any{}
	for at := start; at <= end; at += 60 {
		marker := productionPart(at, "coverage")
		marker["production"] = metric.ProductionCounts{}
		coverageStatus := "complete"
		models := []map[string]any{}
		b, exists := byMinute[at]
		if !exists || !b.Complete {
			reason := "uncollected_minute"
			if exists {
				reason = b.Error
			}
			if at == now.Unix()/60*60 {
				reason = "minute_open"
			}
			coverageStatus = reason
		} else {
			partsByModel := map[string][]map[string]any{}
			stats := map[string]metric.PlatformMetric{}
			latest := map[string]int64{}
			for _, entry := range b.Entries {
				key := fmt.Sprintf("%d\x00%s", entry.ChannelID, entry.ModelName)
				p := productionPart(at, entry.Generation)
				p["production"], p["health"], p["complete"] = entry.Counts, entry.Health, true
				partsByModel[key] = append(partsByModel[key], p)
				row := stats[key]
				row.ChannelID = entry.ChannelID
				row.ModelName = entry.ModelName
				row.Minute = at
				row.TTFTSchema = 2
				row.InputTokens += entry.InputTokens
				row.OutputTokens += entry.OutputTokens
				row.DurationSumMS += entry.DurationSumMS
				if row.TTFTHistogram == nil {
					row.TTFTHistogram = map[string]int64{}
				}
				if entry.LastObservedAt >= latest[key] && len(entry.RecentTTFT) > 0 {
					ewma := float64(entry.RecentTTFT[0])
					for _, value := range entry.RecentTTFT[1:] {
						ewma = .5*float64(value) + .5*ewma
					}
					row.TTFTEWMA5 = ewma
					latest[key] = entry.LastObservedAt
				}
				row.AttemptCount += entry.Counts.N
				row.TotalCount += entry.Counts.N
				row.SuccessCount += entry.Counts.S
				row.FailedCount += entry.Counts.F
				row.FailureAxisCount += entry.Counts.A
				if h := entry.Health; h != nil {
					row.Status429Count += h.HTTP429
					row.Status5xxCount += h.HTTP5xx
					row.FirstTokenCount += h.FirstToken
					row.FirstTokenMissingCount += h.FirstTokenMissing
					row.StreamInterruptedCount += h.StreamInterrupted
					row.RetryCount += h.Retry
					row.TTFTCount += h.TTFTCount
					row.TTFTSum += h.TTFTSum
					for i, bound := range []int64{10, 25, 50, 100, 250, 500, 1000, 2000, 5000, 10000, 30000, 60000, 120000, 300000} {
						label := fmt.Sprintf("le_%d", bound)
						row.TTFTHistogram[label] += h.Histogram[i]
					}
					row.TTFTHistogram["inf"] += h.TTFTCount
				}
				if row.TotalCount > 0 {
					row.SuccessRate = float64(row.SuccessCount) / float64(row.TotalCount)
				}
				if row.TTFTCount > 0 {
					target := (row.TTFTCount*95 + 99) / 100
					row.TTFTP95 = 300000
					for _, bound := range []int64{10, 25, 50, 100, 250, 500, 1000, 2000, 5000, 10000, 30000, 60000, 120000, 300000} {
						if row.TTFTHistogram[fmt.Sprintf("le_%d", bound)] >= target {
							row.TTFTP95 = float64(bound)
							break
						}
					}
				}
				histogram, _ := json.Marshal(row.TTFTHistogram)
				row.TTFTHistogramJSON = string(histogram)
				stats[key] = row
			}
			keys := []string{}
			for key := range partsByModel {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			wireError := false
			for _, key := range keys {
				encoded, e := encodeProductionParts(partsByModel[key])
				if e != nil {
					wireError = true
					break
				}
				statRaw, _ := json.Marshal(stats[key])
				var output map[string]any
				_ = json.Unmarshal(statRaw, &output)
				output["speed_evidence_json"] = encoded
				output["ttft_input_cap_tokens"] = 2000
				models = append(models, output)
			}
			marker["production"], marker["complete"], marker["production_manifest"] = metric.ProductionCounts{}, true, map[string]any{"batch_id": b.BatchID, "entries": b.Entries}
			if _, e := encodeProductionParts([]map[string]any{marker}); e != nil {
				wireError = true
			}
			if wireError {
				models = []map[string]any{}
				marker = productionPart(at, "coverage")
				marker["production"] = metric.ProductionCounts{}
				coverageStatus = "evidence_capacity_exceeded"
			}
		}
		encoded, e := encodeProductionParts([]map[string]any{marker})
		if e != nil {
			return nil, e
		}
		models = append(models, map[string]any{"channel_id": 0, "model_name": metric.CoverageModel, "speed_evidence_json": encoded, "coverage_status": coverageStatus})
		result = append(result, map[string]any{"minute": at, "key": fmt.Sprintf("starhub:%d", at), "models": models})
	}
	return result, nil
}

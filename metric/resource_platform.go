package metric

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"
)

// PlatformMetric is a wire-compatible projection; endpoint names preserve the
// resource platform contract and do not imply Redis or MySQL dependencies.
type PlatformMetric struct {
	ChannelID              int64            `json:"channel_id"`
	ModelName              string           `json:"model_name"`
	Minute                 int64            `json:"minute,omitempty"`
	Key                    string           `json:"key,omitempty"`
	WindowStart            time.Time        `json:"window_start"`
	WindowEnd              time.Time        `json:"window_end"`
	AttemptCount           int64            `json:"attempt_count"`
	TotalCount             int64            `json:"total_count"`
	SuccessCount           int64            `json:"success_count"`
	FailedCount            int64            `json:"failed_count"`
	Status429Count         int64            `json:"status_429_count"`
	Status5xxCount         int64            `json:"status_5xx_count"`
	FailureAxisCount       int64            `json:"failure_axis_count"`
	FirstTokenCount        int64            `json:"first_token_count"`
	FirstTokenMissingCount int64            `json:"first_token_missing_count"`
	StreamInterruptedCount int64            `json:"stream_interrupted_count"`
	RetryCount             int64            `json:"retry_count"`
	TTFTCount              int64            `json:"ttft_count"`
	TTFTSum                int64            `json:"ttft_sum"`
	TTFTP95                float64          `json:"ttft_p95"`
	TTFTEWMA5              float64          `json:"ttft_ewma5"`
	SuccessRate            float64          `json:"success_rate"`
	TTFTHistogram          map[string]int64 `json:"ttft_histogram"`
	TTFTHistogramJSON      string           `json:"ttft_histogram_json"`
}

type platformMetricKey struct {
	channel int64
	model   string
	minute  int64
}

var platformMetrics = struct {
	sync.Mutex
	rows map[platformMetricKey]*PlatformMetric
}{rows: map[platformMetricKey]*PlatformMetric{}}

func recordPlatformAttempt(meta RequestMeta, a *AttemptSpan, retry bool) {
	if a.outcome == AttemptUnsupported || (a.outcome == AttemptClientError && a.statusCode != 429 && a.statusCode != 401 && a.statusCode != 403 && a.statusCode != 404) {
		return
	}
	now := time.Now().Unix()
	minute := now / 60 * 60
	platformMetrics.Lock()
	defer platformMetrics.Unlock()
	for key := range platformMetrics.rows {
		if key.minute < minute-86400 {
			delete(platformMetrics.rows, key)
		}
	}
	key := platformMetricKey{a.channelID, meta.Model, minute}
	row := platformMetrics.rows[key]
	if row == nil {
		// Keep cardinality bounded even if many new models are configured.
		if len(platformMetrics.rows) >= 100000 {
			return
		}
		row = &PlatformMetric{ChannelID: a.channelID, ModelName: meta.Model, Minute: minute, TTFTHistogram: map[string]int64{}}
		platformMetrics.rows[key] = row
	}
	row.AttemptCount++
	row.TotalCount++
	if a.outcome == AttemptSuccess {
		row.SuccessCount++
	} else {
		row.FailedCount++
		row.FailureAxisCount++
	}
	if a.statusCode == 429 {
		row.Status429Count++
	}
	if a.statusCode >= 500 {
		row.Status5xxCount++
	}
	if retry {
		row.RetryCount++
	}
	// Header latency is not a first content event. Never label it as streaming TTFT.
	if a.hasFirstToken || (!meta.Stream && a.ttft > 0) {
		ms := max(int64(1), a.ttft.Milliseconds())
		row.FirstTokenCount++
		row.TTFTCount++
		row.TTFTSum += ms
		bound := int64(1)
		for bound < ms && bound < 131072 {
			bound *= 2
		}
		row.TTFTHistogram[strconv.FormatInt(bound, 10)]++
		if row.TTFTCount == 1 {
			row.TTFTEWMA5 = float64(ms)
		} else {
			row.TTFTEWMA5 += (float64(ms) - row.TTFTEWMA5) / 3
		}
	} else if meta.Stream {
		row.FirstTokenMissingCount++
	}
}

func recordPlatformInterruption(meta RequestMeta, a *AttemptSpan) {
	if a == nil {
		return
	}
	platformMetrics.Lock()
	defer platformMetrics.Unlock()
	key := platformMetricKey{a.channelID, meta.Model, a.start.Unix() / 60 * 60}
	if row := platformMetrics.rows[key]; row != nil {
		row.StreamInterruptedCount++
	}
}

func addPlatformMetric(dst *PlatformMetric, src PlatformMetric) {
	dst.AttemptCount += src.AttemptCount
	dst.TotalCount += src.TotalCount
	dst.SuccessCount += src.SuccessCount
	dst.FailedCount += src.FailedCount
	dst.Status429Count += src.Status429Count
	dst.Status5xxCount += src.Status5xxCount
	dst.FailureAxisCount += src.FailureAxisCount
	dst.FirstTokenCount += src.FirstTokenCount
	dst.FirstTokenMissingCount += src.FirstTokenMissingCount
	dst.StreamInterruptedCount += src.StreamInterruptedCount
	dst.RetryCount += src.RetryCount
	dst.TTFTCount += src.TTFTCount
	dst.TTFTSum += src.TTFTSum
	if src.TTFTCount > 0 {
		dst.TTFTEWMA5 = src.TTFTEWMA5
	}
	for bound, count := range src.TTFTHistogram {
		dst.TTFTHistogram[bound] += count
	}
}

func finishPlatformMetric(row *PlatformMetric) {
	if row.TotalCount > 0 {
		row.SuccessRate = 100 * float64(row.SuccessCount) / float64(row.TotalCount)
	}
	bounds := make([]int, 0, len(row.TTFTHistogram))
	for bound := range row.TTFTHistogram {
		v, _ := strconv.Atoi(bound)
		bounds = append(bounds, v)
	}
	sort.Ints(bounds)
	var total int64
	for _, bound := range bounds {
		total += row.TTFTHistogram[strconv.Itoa(bound)]
		if total >= (row.TTFTCount*95+99)/100 {
			row.TTFTP95 = float64(bound)
			break
		}
	}
	raw, _ := json.Marshal(row.TTFTHistogram)
	row.TTFTHistogramJSON = string(raw)
}

func PlatformMetricRows(byModel, quarterHour bool, start, end int64) []PlatformMetric {
	platformMetrics.Lock()
	defer platformMetrics.Unlock()
	keys := make([]platformMetricKey, 0, len(platformMetrics.rows))
	for key := range platformMetrics.rows {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].minute != keys[j].minute {
			return keys[i].minute < keys[j].minute
		}
		if keys[i].channel != keys[j].channel {
			return keys[i].channel < keys[j].channel
		}
		return keys[i].model < keys[j].model
	})
	aggregated := map[platformMetricKey]*PlatformMetric{}
	width := int64(60)
	if quarterHour {
		width = 900
	}
	for _, sourceKey := range keys {
		if sourceKey.minute < start || sourceKey.minute > end {
			continue
		}
		key := sourceKey
		key.minute = key.minute / width * width
		if !byModel {
			key.channel, key.model = 0, ""
		}
		row := aggregated[key]
		if row == nil {
			row = &PlatformMetric{ChannelID: key.channel, ModelName: key.model, Minute: key.minute, Key: fmt.Sprintf("starhub:%d", key.minute), WindowStart: time.Unix(key.minute, 0).UTC(), WindowEnd: time.Unix(key.minute+width, 0).UTC(), TTFTHistogram: map[string]int64{}}
			aggregated[key] = row
		}
		addPlatformMetric(row, *platformMetrics.rows[sourceKey])
	}
	result := make([]PlatformMetric, 0, len(aggregated))
	for _, row := range aggregated {
		finishPlatformMetric(row)
		result = append(result, *row)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Minute != result[j].Minute {
			return result[i].Minute < result[j].Minute
		}
		if result[i].ChannelID != result[j].ChannelID {
			return result[i].ChannelID < result[j].ChannelID
		}
		return result[i].ModelName < result[j].ModelName
	})
	return result
}

func resetPlatformMetrics() {
	platformMetrics.Lock()
	defer platformMetrics.Unlock()
	platformMetrics.rows = map[platformMetricKey]*PlatformMetric{}
}

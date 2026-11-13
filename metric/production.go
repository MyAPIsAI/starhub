package metric

import (
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

const ProductionContract = "confirmed-production-attempts-v2"
const CoverageModel = "__production_coverage_v2__"

type AttemptMetadata struct {
	RequestedCapabilities []string `json:"requested_capabilities,omitempty"`
	ResourceKey           string   `json:"resource_key"`
	Generation            string   `json:"endpoint_generation"`
	Protocol              string   `json:"protocol"`
	Capability            string   `json:"capability"`
	Group                 string   `json:"group"`
	Affinity              string   `json:"affinity"`
}

// ProductionAttempt contains routing and metering metadata only, never input,
// response content, credential material, customer identity or prompt hashes.
type ProductionAttempt struct {
	AttemptMetadata
	ID             string `json:"attempt_id"`
	ChannelID      int64  `json:"channel_id"`
	Model          string `json:"model"`
	StartedAt      int64  `json:"started_at"`
	EndedAt        int64  `json:"ended_at"`
	Outcome        string `json:"outcome"`
	Status         int    `json:"status"`
	InputTokens    int64  `json:"input_tokens"`
	OutputTokens   int64  `json:"output_tokens"`
	DurationMS     int64  `json:"duration_ms"`
	FirstContentMS int64  `json:"first_content_ms"`
	Stream         bool   `json:"stream"`
	Retry          bool   `json:"retry"`
}

type ProductionCounts struct {
	N int64 `json:"n"`
	S int64 `json:"s"`
	F int64 `json:"f"`
	A int64 `json:"a"`
}
type ProductionHealth struct {
	Contract          string    `json:"contract"`
	TimingUnknown     int64     `json:"timing_unknown"`
	N                 int64     `json:"n"`
	HTTP429           int64     `json:"http429"`
	HTTP5xx           int64     `json:"http5xx"`
	FirstToken        int64     `json:"first_token"`
	FirstTokenMissing int64     `json:"first_token_missing"`
	StreamInterrupted int64     `json:"stream_interrupted"`
	Retry             int64     `json:"retry"`
	InputCap          int       `json:"input_cap"`
	TTFTCount         int64     `json:"ttft_count"`
	TTFTSum           int64     `json:"ttft_sum"`
	Histogram         [14]int64 `json:"histogram"`
}
type ProductionEntry struct {
	InputTokens    int64             `json:"input_tokens"`
	OutputTokens   int64             `json:"output_tokens"`
	DurationSumMS  int64             `json:"duration_sum_ms"`
	RecentTTFT     []int64           `json:"recent_ttft,omitempty"`
	LastObservedAt int64             `json:"last_observed_at"`
	ChannelID      int64             `json:"channel_id"`
	ModelName      string            `json:"model_name"`
	Generation     string            `json:"endpoint_generation"`
	Counts         ProductionCounts  `json:"counts"`
	Health         *ProductionHealth `json:"health,omitempty"`
}
type ProductionMinute struct {
	Minute   int64               `json:"minute"`
	BatchID  string              `json:"batch_id"`
	Complete bool                `json:"complete"`
	Error    string              `json:"error,omitempty"`
	Entries  []ProductionEntry   `json:"entries"`
	Attempts []ProductionAttempt `json:"attempts"`
}

var production = struct {
	sync.Mutex
	started  int64
	next     int64
	sealed   int64
	retained int
	buckets  map[int64]*ProductionMinute
}{buckets: map[int64]*ProductionMinute{}}

// StartProduction begins a new collection epoch. Its first partial minute is
// explicitly incomplete; an unclean restart can never manufacture zero traffic.
func StartProduction(now time.Time) {
	production.Lock()
	defer production.Unlock()
	production.started = now.Unix() / 60 * 60
	production.next, production.sealed = production.started, production.started
	production.retained = 0
	production.buckets = map[int64]*ProductionMinute{}
	b := productionBucket(production.started)
	b.Complete, b.Error = false, "collector_started_mid_minute"
}

func productionBucket(minute int64) *ProductionMinute {
	b := production.buckets[minute]
	if b == nil {
		b = &ProductionMinute{Minute: minute, BatchID: uuid.NewString(), Complete: true, Entries: []ProductionEntry{}, Attempts: []ProductionAttempt{}}
		production.buckets[minute] = b
	}
	return b
}

func (a *AttemptSpan) SetRouteMetadata(meta AttemptMetadata) {
	if a != nil {
		a.route = meta
	}
}

func (a *AttemptSpan) MarkSent() {
	if a != nil {
		a.sent = true
	}
}

func recordProductionAttempt(meta RequestMeta, a *AttemptSpan, retry bool) {
	production.Lock()
	defer production.Unlock()
	if production.started == 0 {
		return
	}
	// Assign completion to the collector's current minute while holding its
	// lock; minute sealing and arrival cannot race to change an exported batch.
	now := time.Now()
	minute := max(now.Unix()/60*60, production.sealed)
	b := productionBucket(minute)
	if a.route.Generation == "" {
		b.Complete = false
		b.Error = "missing_endpoint_identity"
		return
	}
	if production.retained >= 100000 || len(b.Entries) >= 4096 {
		b.Complete = false
		b.Error = "collector_capacity_exceeded"
		return
	}
	outcome := "failure"
	if a.outcome == AttemptSuccess {
		outcome = "success"
	} else if a.outcome == AttemptUnsupported {
		return
	} else if a.outcome == AttemptUncertain {
		outcome = "uncertain"
	}
	if !a.sent {
		outcome = "unsent"
	}
	event := ProductionAttempt{AttemptMetadata: a.route, ID: uuid.NewString(), ChannelID: a.channelID, Model: meta.Model, StartedAt: a.start.UnixMilli(), EndedAt: now.UnixMilli(), Outcome: outcome, Status: a.statusCode, InputTokens: a.inputTokens, OutputTokens: a.outputTokens, DurationMS: max(0, a.endedAt.Sub(a.start).Milliseconds()), Stream: meta.Stream, Retry: retry}
	if a.hasFirstToken {
		event.FirstContentMS = max(1, a.ttft.Milliseconds())
	}
	b.Attempts = append(b.Attempts, event)
	production.retained++
	// Customer validation errors and uncertain outcomes remain in the attempt
	// journal but cannot become confirmed provider failures or successes.
	if outcome == "uncertain" || outcome == "unsent" || (a.outcome == AttemptClientError && a.statusCode != 401 && a.statusCode != 403 && a.statusCode != 404 && a.statusCode != 429) {
		return
	}
	index := -1
	for i := range b.Entries {
		e := &b.Entries[i]
		if e.ChannelID == a.channelID && e.ModelName == meta.Model && e.Generation == a.route.Generation {
			index = i
			break
		}
	}
	if index < 0 {
		b.Entries = append(b.Entries, ProductionEntry{ChannelID: a.channelID, ModelName: meta.Model, Generation: a.route.Generation, Health: &ProductionHealth{Contract: "confirmed-production-health-v1", InputCap: 2000}})
		index = len(b.Entries) - 1
	}
	e := &b.Entries[index]
	e.InputTokens += a.inputTokens
	e.OutputTokens += a.outputTokens
	e.DurationSumMS += event.DurationMS
	e.LastObservedAt = now.UnixMilli()
	e.Counts.N++
	if outcome == "success" {
		e.Counts.S++
	} else {
		e.Counts.F++
		if a.statusCode == 401 || a.statusCode == 403 || a.statusCode == 404 {
			e.Counts.A++
		}
	}
	h := e.Health
	h.N++
	if a.statusCode == 429 {
		h.HTTP429++
	}
	if a.statusCode >= 500 {
		h.HTTP5xx++
	}
	if retry {
		h.Retry++
	}
	if a.hasFirstToken {
		h.FirstToken++
	} else if meta.Stream {
		h.FirstTokenMissing++
	} else {
		h.TimingUnknown++
	}
	if a.interrupted && a.hasFirstToken {
		h.StreamInterrupted++
	}
	if a.hasFirstToken && a.inputTokens > 0 && a.inputTokens <= 2000 && outcome == "success" {
		ms := max(1, a.ttft.Milliseconds())
		h.TTFTCount++
		h.TTFTSum += ms
		e.RecentTTFT = append(e.RecentTTFT, ms)
		if len(e.RecentTTFT) > 5 {
			e.RecentTTFT = e.RecentTTFT[len(e.RecentTTFT)-5:]
		}
		for i, bound := range platformTTFTBounds {
			if ms <= bound {
				h.Histogram[i]++
			}
		}
	}
}

// ClosedProductionMinutes returns detached, immutable batches. A failed database
// write is retried with the same IDs; AckProduction frees memory only on commit.
func ClosedProductionMinutes(now time.Time) []ProductionMinute {
	production.Lock()
	defer production.Unlock()
	if production.started == 0 {
		return nil
	}
	end := now.Unix() / 60 * 60
	for at, b := range production.buckets {
		if at < end-86400 {
			production.retained -= len(b.Attempts)
			delete(production.buckets, at)
		}
	}
	start := max(production.next, end-86400)
	result := []ProductionMinute{}
	for at := start; at < end; at += 60 {
		b := productionBucket(at)
		sort.Slice(b.Entries, func(i, j int) bool {
			a, c := b.Entries[i], b.Entries[j]
			if a.ChannelID != c.ChannelID {
				return a.ChannelID < c.ChannelID
			}
			if a.ModelName != c.ModelName {
				return a.ModelName < c.ModelName
			}
			return a.Generation < c.Generation
		})
		raw, _ := json.Marshal(b)
		var copy ProductionMinute
		_ = json.Unmarshal(raw, &copy)
		result = append(result, copy)
	}
	production.sealed = max(production.sealed, end)
	return result
}

func AckProduction(batch []ProductionMinute) {
	production.Lock()
	defer production.Unlock()
	for _, b := range batch {
		if current := production.buckets[b.Minute]; current != nil && current.BatchID == b.BatchID {
			production.retained -= len(current.Attempts)
			delete(production.buckets, b.Minute)
			production.next = max(production.next, b.Minute+60)
		}
	}
}

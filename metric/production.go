package metric

import (
	"github.com/google/uuid"
	"sync"
	"sync/atomic"
	"time"
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

type productionEntryKey struct {
	channel           int64
	model, generation string
}

var production = struct {
	sync.RWMutex
	started, next, sealed int64
	retained              atomic.Int64
	buckets               map[int64]*productionBuffer
}{buckets: map[int64]*productionBuffer{}}

// StartProduction marks this process's first partial minute as incomplete.
func StartProduction(now time.Time) {
	production.Lock()
	defer production.Unlock()
	production.started = now.Unix() / 60 * 60
	production.next = production.started
	production.sealed = production.started
	production.retained.Store(0)
	production.buckets = map[int64]*productionBuffer{}
	productionBucket(production.started).markLoss("collector_started_mid_minute")
}

func productionBucket(minute int64) *productionBuffer {
	b := production.buckets[minute]
	if b == nil {
		b = newProductionBuffer(minute)
		production.buckets[minute] = b
	}
	return b
}

// A read lock fences a writer against minute rotation without serializing
// unrelated resources. Only first access to a new minute needs the write lock.
func lockProductionBuffer() *productionBuffer {
	for {
		production.RLock()
		if production.started == 0 {
			production.RUnlock()
			return nil
		}
		minute := max(time.Now().Unix()/60*60, production.sealed)
		if b := production.buckets[minute]; b != nil {
			return b
		}
		production.RUnlock()
		production.Lock()
		if production.started != 0 {
			productionBucket(max(time.Now().Unix()/60*60, production.sealed))
		}
		production.Unlock()
	}
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
	b := lockProductionBuffer()
	if b == nil {
		return
	}
	defer production.RUnlock()
	if a.route.Generation == "" {
		b.markLoss("missing_endpoint_identity")
		return
	}
	key := productionEntryKey{a.channelID, meta.Model, a.route.Generation}
	shard := &b.shards[productionShardIndex(key)]
	shard.Lock()
	defer shard.Unlock()
	index, exists := shard.index[key]
	if !exists && b.entryCount.Add(1) > 4096 {
		b.entryCount.Add(-1)
		b.markLoss("collector_capacity_exceeded")
		return
	}
	reserved := false
	for {
		n := production.retained.Load()
		if n >= 100000 {
			break
		}
		if production.retained.CompareAndSwap(n, n+1) {
			reserved = true
			break
		}
	}
	if !reserved {
		if !exists {
			b.entryCount.Add(-1)
		}
		b.markLoss("collector_capacity_exceeded")
		return
	}
	now := time.Now()
	outcome := "failure"
	if a.outcome == AttemptSuccess {
		outcome = "success"
	} else if a.outcome == AttemptUnsupported {
		production.retained.Add(-1)
		if !exists {
			b.entryCount.Add(-1)
		}
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
	event.RequestedCapabilities = append([]string(nil), event.RequestedCapabilities...)
	shard.attempts = append(shard.attempts, event)
	b.count.Add(1)
	// Customer validation errors and uncertain outcomes remain in the attempt
	// journal but cannot become confirmed provider failures or successes.
	if outcome == "uncertain" || outcome == "unsent" || (a.outcome == AttemptClientError && a.statusCode != 401 && a.statusCode != 403 && a.statusCode != 404 && a.statusCode != 429) {
		if !exists {
			b.entryCount.Add(-1)
		}
		return
	}
	if !exists {
		shard.entries = append(shard.entries, ProductionEntry{ChannelID: a.channelID, ModelName: meta.Model, Generation: a.route.Generation, Health: &ProductionHealth{Contract: "confirmed-production-health-v1", InputCap: 2000}})
		index = len(shard.entries) - 1
		shard.index[key] = index
	}
	e := &shard.entries[index]
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
	if production.started == 0 {
		production.Unlock()
		return nil
	}
	end := now.Unix() / 60 * 60
	for at, b := range production.buckets {
		if at < end-86400 {
			production.retained.Add(-b.count.Load())
			delete(production.buckets, at)
		}
	}
	buffers := []*productionBuffer{}
	for at := max(production.next, end-86400); at < end; at += 60 {
		buffers = append(buffers, productionBucket(at))
	}
	production.sealed = max(production.sealed, end)
	production.Unlock()
	result := make([]ProductionMinute, 0, len(buffers))
	for _, b := range buffers {
		result = append(result, b.snapshot())
	}
	return result
}

func AckProduction(batch []ProductionMinute) {
	production.Lock()
	defer production.Unlock()
	for _, b := range batch {
		if current := production.buckets[b.Minute]; current != nil && current.view.BatchID == b.BatchID {
			production.retained.Add(-current.count.Load())
			delete(production.buckets, b.Minute)
			production.next = max(production.next, b.Minute+60)
		}
	}
}

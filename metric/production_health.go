package metric

import "sync/atomic"

const ProductionMemoryBudget int64 = 128 << 20

var productionRejected, productionExpired, productionExpiredBytes atomic.Int64
var productionCollected atomic.Int64

type ProductionCollectorHealth struct {
	StartedAt         int64 `json:"started_at"`
	Retained          int64 `json:"retained_attempts"`
	EstimatedBytes    int64 `json:"estimated_bytes"`
	MaxBytes          int64 `json:"max_bytes"`
	MaxAttempts       int64 `json:"max_attempts"`
	Buckets           int   `json:"buckets"`
	IncompleteBuckets int   `json:"incomplete_buckets"`
	OldestMinute      int64 `json:"oldest_minute"`
	Collected         int64 `json:"collected_attempts"`
	Rejected          int64 `json:"rejected_attempts"`
	Expired           int64 `json:"expired_attempts"`
	ExpiredBytes      int64 `json:"expired_estimated_bytes"`
}

func ProductionCollectorSnapshot() ProductionCollectorHealth {
	production.RLock()
	defer production.RUnlock()
	h := ProductionCollectorHealth{StartedAt: production.started, Retained: production.retained.Load(), EstimatedBytes: production.bytes.Load(), MaxBytes: ProductionMemoryBudget, MaxAttempts: 100000, Buckets: len(production.buckets), Collected: productionCollected.Load(), Rejected: productionRejected.Load(), Expired: productionExpired.Load(), ExpiredBytes: productionExpiredBytes.Load()}
	for at, b := range production.buckets {
		if h.OldestMinute == 0 || at < h.OldestMinute {
			h.OldestMinute = at
		}
		if b.loss.Load() != nil {
			h.IncompleteBuckets++
		}
	}
	return h
}

// The estimate covers object overhead and worst-case JSON escaping. It is an
// admission budget, not a measurement of Go heap or SQLite file size.
func productionAttemptBytes(meta RequestMeta, route AttemptMetadata) int64 {
	n := len(meta.Model) + len(route.ResourceKey) + len(route.Generation) + len(route.Protocol) + len(route.Capability) + len(route.Group) + len(route.Affinity)
	for _, v := range route.RequestedCapabilities {
		n += len(v)
	}
	return 1024 + 6*int64(n) + 64*int64(len(route.RequestedCapabilities))
}

func reserveProductionBytes(n int64) bool {
	for {
		used := production.bytes.Load()
		if n > ProductionMemoryBudget-used {
			return false
		}
		if production.bytes.CompareAndSwap(used, used+n) {
			return true
		}
	}
}

package metric

import (
	"sync/atomic"
	"time"
)

var durationBounds = [...]int64{10, 50, 100, 250, 500, 1000, 2500, 5000, 10000, 25000, 50000, 100000, 250000, 500000, 1000000, 5000000, 10000000}

// DurationHistogram has fixed memory and reports approximate upper-bound
// quantiles in microseconds. Overflow uses the observed maximum.
type DurationHistogram struct {
	bins                  [18]atomic.Int64
	count, total, maximum atomic.Int64
}

type DurationSnapshot struct {
	Count   int64 `json:"count"`
	TotalUS int64 `json:"total_us"`
	MaxUS   int64 `json:"max_us"`
	P95US   int64 `json:"p95_us"`
	P99US   int64 `json:"p99_us"`
}

func (h *DurationHistogram) Observe(d time.Duration) {
	us := max(0, d.Microseconds())
	i := 0
	for i < len(durationBounds) && us > durationBounds[i] {
		i++
	}
	h.bins[i].Add(1)
	h.total.Add(us)
	for old := h.maximum.Load(); us > old; old = h.maximum.Load() {
		if h.maximum.CompareAndSwap(old, us) {
			break
		}
	}
	h.count.Add(1)
}

func (h *DurationHistogram) Snapshot() DurationSnapshot {
	s := DurationSnapshot{Count: h.count.Load(), TotalUS: h.total.Load(), MaxUS: h.maximum.Load()}
	var cumulative int64
	for i := range h.bins {
		cumulative += h.bins[i].Load()
		bound := s.MaxUS
		if i < len(durationBounds) {
			bound = durationBounds[i]
		}
		if s.Count > 0 && s.P95US == 0 && cumulative >= (s.Count*95+99)/100 {
			s.P95US = bound
		}
		if s.Count > 0 && s.P99US == 0 && cumulative >= (s.Count*99+99)/100 {
			s.P99US = bound
		}
	}
	return s
}

var collectorDuration, collectorLockWait, collectorLockHold, collectorShardWait, collectorShardHold, collectorSealDuration DurationHistogram

func ProductionTimings() map[string]DurationSnapshot {
	return map[string]DurationSnapshot{"collect": collectorDuration.Snapshot(), "lifecycle_wait": collectorLockWait.Snapshot(), "lifecycle_hold": collectorLockHold.Snapshot(), "shard_wait": collectorShardWait.Snapshot(), "shard_hold": collectorShardHold.Snapshot(), "seal": collectorSealDuration.Snapshot()}
}

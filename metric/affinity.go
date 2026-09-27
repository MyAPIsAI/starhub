package metric

import "sync/atomic"

// AffinityEvent classifies one sticky-routing observation. The counters are
// intentionally aggregate-only (no token/model labels) so cardinality and
// privacy stay bounded.
type AffinityEvent int

const (
	// AffinityHit: the affinity preference was honored for the first pick.
	AffinityHit AffinityEvent = iota
	// AffinityMiss: affinity was eligible but no usable preference existed.
	AffinityMiss
	// AffinityPreferredFailed: the preferred channel failed with a retryable
	// error and was cleared.
	AffinityPreferredFailed
	// AffinityFallbackSuccess: a different channel succeeded after the
	// preferred one failed.
	AffinityFallbackSuccess
	// AffinityWrite: a successful attempt refreshed the preference.
	AffinityWrite
	// AffinityClear: a retryable failure removed the preference.
	AffinityClear
	numAffinityEvents
)

var affinityOrder = []AffinityEvent{
	AffinityHit, AffinityMiss, AffinityPreferredFailed,
	AffinityFallbackSuccess, AffinityWrite, AffinityClear,
}

var affinityCounts [numAffinityEvents]atomic.Int64

// String renders the event for the admin API.
func (e AffinityEvent) String() string {
	switch e {
	case AffinityHit:
		return "hit"
	case AffinityMiss:
		return "miss"
	case AffinityPreferredFailed:
		return "preferred_failed"
	case AffinityFallbackSuccess:
		return "fallback_success"
	case AffinityWrite:
		return "write"
	case AffinityClear:
		return "clear"
	}
	return "unknown"
}

// RecordAffinity bumps the counter for one affinity observation. The hot relay
// path uses atomic counters so it never contends on a mutex.
func RecordAffinity(e AffinityEvent) {
	if e < 0 || e >= numAffinityEvents {
		return
	}
	affinityCounts[e].Add(1)
}

// AffinitySnapshot is the counter summary for one affinity event.
type AffinitySnapshot struct {
	Event string `json:"event"`
	Count int64  `json:"count"`
}

func affinitySnapshots() []AffinitySnapshot {
	out := make([]AffinitySnapshot, 0, len(affinityOrder))
	for _, e := range affinityOrder {
		out = append(out, AffinitySnapshot{Event: e.String(), Count: affinityCounts[e].Load()})
	}
	return out
}

// resetAffinity clears the affinity counters (tests only).
func resetAffinity() {
	for i := range affinityCounts {
		affinityCounts[i].Store(0)
	}
}

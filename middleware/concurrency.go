package middleware

import (
	"net/http"
	"sync"
	"sync/atomic"

	"starhub/common"
)

// ConcurrencyLimiter tracks in-flight requests per user using atomic counters.
// Default is 50 concurrent requests per account; 0 means unlimited.
var (
	concurrencyMu      sync.RWMutex
	maxConcurrent      int64    = 50
	concurrentCounters sync.Map // userID (int64) -> *atomic.Int64
	globalInFlight     atomic.Int64
	maxGlobal          int64 = 0 // 0 = unlimited
)

// ConfigureConcurrency sets the per-account and global concurrency limits.
func ConfigureConcurrency(perAccount, global int64) {
	concurrencyMu.Lock()
	defer concurrencyMu.Unlock()
	maxConcurrent = perAccount
	maxGlobal = global
}

// GetMaxConcurrent returns the current per-account limit.
func GetMaxConcurrent() int64 {
	concurrencyMu.RLock()
	defer concurrencyMu.RUnlock()
	return maxConcurrent
}

// ConcurrencyLimit middleware enforces per-account and global in-flight
// request limits. Requests exceeding the limit receive 429. The counter
// is decremented via defer when the request completes, so stream
// completions and cancellations reliably release the slot.
func ConcurrencyLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		concurrencyMu.RLock()
		perAccount := maxConcurrent
		global := maxGlobal
		concurrencyMu.RUnlock()

		// Global check.
		if global > 0 && globalInFlight.Load() >= global {
			common.ApiError(w, http.StatusTooManyRequests, "platform is at maximum concurrency; retry later")
			return
		}

		// Per-account check.
		if perAccount > 0 {
			userID, _ := r.Context().Value(common.CtxUserID).(int64)
			if userID > 0 {
				val, _ := concurrentCounters.LoadOrStore(userID, &atomic.Int64{})
				counter := val.(*atomic.Int64)
				if counter.Load() >= perAccount {
					common.ApiError(w, http.StatusTooManyRequests, "account concurrency limit exceeded")
					return
				}
				counter.Add(1)
				globalInFlight.Add(1)
				defer func() {
					counter.Add(-1)
					globalInFlight.Add(-1)
				}()
			}
		}
		next.ServeHTTP(w, r)
	})
}

// ResetConcurrency clears all counters (used by tests).
func ResetConcurrency() {
	concurrentCounters.Range(func(key, val any) bool {
		concurrentCounters.Delete(key)
		return true
	})
	globalInFlight.Store(0)
}

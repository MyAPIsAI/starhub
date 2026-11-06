package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"starhub/common"
)

func TestConcurrencyLimitAllowsUnderLimit(t *testing.T) {
	ResetConcurrency()
	ConfigureConcurrency(5, 0)
	defer ConfigureConcurrency(50, 0)

	called := false
	handler := ConcurrencyLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/channel", nil)
	ctx := common.WithValues(req.Context(), common.CtxUserID, int64(1))
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if !called {
		t.Fatal("handler should be called when under limit")
	}
}

func TestConcurrencyLimitBlocksOverLimit(t *testing.T) {
	ResetConcurrency()
	ConfigureConcurrency(2, 0)
	defer ConfigureConcurrency(50, 0)

	handler := ConcurrencyLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	// Block 2 slots by not releasing them.
	req := httptest.NewRequest(http.MethodGet, "/api/channel", nil)
	ctx := common.WithValues(req.Context(), common.CtxUserID, int64(1))
	req = req.WithContext(ctx)

	// Simulate 2 in-flight requests by manually incrementing counters.
	val, _ := concurrentCounters.LoadOrStore(int64(1), new(atomic.Int64))
	counter := val.(*atomic.Int64)
	counter.Add(2)

	// Third request should be blocked.
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rr.Code)
	}

	// Release one slot.
	counter.Add(-1)
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusOK {
		t.Fatalf("expected 200 after releasing a slot, got %d", rr2.Code)
	}
}

func TestConcurrencyLimitNoUserIDPassesThrough(t *testing.T) {
	ResetConcurrency()
	ConfigureConcurrency(1, 0)

	called := false
	handler := ConcurrencyLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/channel", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if !called {
		t.Fatal("handler should be called when no user ID in context")
	}
}

func TestConcurrencyLimitZeroMeansUnlimited(t *testing.T) {
	ResetConcurrency()
	ConfigureConcurrency(0, 0)

	called := false
	handler := ConcurrencyLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/channel", nil)
	ctx := common.WithValues(req.Context(), common.CtxUserID, int64(1))
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if !called {
		t.Fatal("handler should be called when limit is 0 (unlimited)")
	}
}

func TestConcurrencyLimitGlobalLimit(t *testing.T) {
	ResetConcurrency()
	ConfigureConcurrency(0, 1) // global limit 1, no per-account
	defer ConfigureConcurrency(50, 0)

	// Occupy the global slot.
	globalInFlight.Add(1)

	called := false
	handler := ConcurrencyLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/channel", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if called {
		t.Fatal("handler should NOT be called when global limit is reached")
	}
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rr.Code)
	}

	// Release and retry.
	globalInFlight.Add(-1)
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusOK {
		t.Fatalf("expected 200 after global slot released, got %d", rr2.Code)
	}
}

func TestConcurrencyLimitReleasesOnPanic(t *testing.T) {
	ResetConcurrency()
	ConfigureConcurrency(1, 0)
	defer ConfigureConcurrency(50, 0)

	handler := ConcurrencyLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/channel", nil)
	ctx := common.WithValues(req.Context(), common.CtxUserID, int64(1))
	req = req.WithContext(ctx)

	// Use a recover wrapper so the test doesn't crash.
	recoverHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { _ = recover() }()
		handler.ServeHTTP(w, r)
	})
	rr := httptest.NewRecorder()
	recoverHandler.ServeHTTP(rr, req)

	// Counter should be back to 0 after the panic.
	val, _ := concurrentCounters.Load(int64(1))
	counter := val.(*atomic.Int64)
	if counter.Load() != 0 {
		t.Fatalf("counter should be 0 after panic, got %d", counter.Load())
	}
}

// Ensure sync is imported.
var _ = sync.Mutex{}

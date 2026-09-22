// Package metric collects relay observability data at three layers:
//
//   - Request level (BeginRequest/EndRequest): status code, retry count,
//     success / stream-interrupt classification.
//   - Attempt level (BeginAttempt): per upstream call TTFT (time to first
//     token) and the upstream request-id, so "which channel is slow" is
//     answerable.
//   - Channel level: rolling success rate, TTFT and duration windows,
//     exposed via Snapshot for the admin surface.
//
// All state is in-memory and process-local; the registry resets on
// restart. It is safe for concurrent use.
package metric

import (
	"sync"
	"sync/atomic"
	"time"
)

// RequestClass classifies the outcome of a whole relay request.
type RequestClass int

const (
	ClassSuccess RequestClass = iota
	ClassClientError
	ClassUpstreamError
	ClassNoChannel
	ClassStreamInterrupted
	ClassInternalError
)

// String renders the class for logs and the admin API.
func (c RequestClass) String() string {
	switch c {
	case ClassSuccess:
		return "success"
	case ClassClientError:
		return "client_error"
	case ClassUpstreamError:
		return "upstream_error"
	case ClassNoChannel:
		return "no_channel"
	case ClassStreamInterrupted:
		return "stream_interrupted"
	case ClassInternalError:
		return "internal_error"
	}
	return "unknown"
}

// AttemptClass classifies the outcome of one upstream attempt.
type AttemptClass int

const (
	AttemptSuccess AttemptClass = iota
	AttemptClientError
	AttemptUpstreamError
	AttemptUnsupported
)

// RequestMeta describes the request being measured.
type RequestMeta struct {
	UserID  int64
	TokenID int64
	Model   string
	Group   string
	Stream  bool
	Mode    string // chat, embeddings, rerank, tts, stt, image, task
}

// RequestSpan tracks one relay request. It is owned by the relayFlow and
// is not meant to be shared across goroutines (the flow itself is
// single-threaded per request); the global registry it reports to is
// concurrency-safe.
type RequestSpan struct {
	meta       RequestMeta
	start      time.Time
	attempts   []*AttemptSpan
	streamGone bool // client disconnected during streaming
}

// AttemptSpan tracks one upstream attempt.
type AttemptSpan struct {
	channelID     int64
	start         time.Time
	upstreamReqID string
	headerTime    time.Duration // request send -> response headers
	ttft          time.Duration  // time to first token (headerTime when not streamed)
	hasFirstToken bool
	ended         bool
	outcome       AttemptClass
	statusCode    int
}

// BeginRequest opens a request-level span. Always pair with EndRequest
// (typically via defer).
func BeginRequest(meta RequestMeta) *RequestSpan {
	return &RequestSpan{meta: meta, start: time.Now()}
}

// BeginAttempt opens an attempt-level span for one channel. The returned
// handle is ended by EndAttempt.
func (s *RequestSpan) BeginAttempt(channelID int64) *AttemptSpan {
	a := &AttemptSpan{channelID: channelID, start: time.Now()}
	s.attempts = append(s.attempts, a)
	return a
}

// MarkStreamInterrupted flags the request as a stream the client
// abandoned mid-flight (the upstream may still have been drained for
// billing, but the user-visible outcome is an interruption).
func (s *RequestSpan) MarkStreamInterrupted() {
	s.streamGone = true
}

// RecordHeaders is called when the upstream response headers arrive. The
// upstream request-id is captured from the response headers so an
// operator can correlate with the provider's own logs.
func (a *AttemptSpan) RecordHeaders(upstreamReqID string) {
	a.headerTime = time.Since(a.start)
	a.upstreamReqID = upstreamReqID
}

// RecordFirstToken is called when the first token is emitted to the
// client (streaming). For non-streaming attempts RecordHeaders already
// sets the TTFT; this refines it to the true client-perceived first
// token. It is a no-op after the first call.
func (a *AttemptSpan) RecordFirstToken() {
	if a.hasFirstToken {
		return
	}
	a.hasFirstToken = true
	a.ttft = time.Since(a.start)
}

// EndAttempt finalizes the attempt and pushes a sample to the channel
// rolling window. Safe to call once; subsequent calls are no-ops.
func (s *RequestSpan) EndAttempt(a *AttemptSpan, outcome AttemptClass, statusCode int) {
	if a == nil || a.ended {
		return
	}
	a.ended = true
	a.outcome = outcome
	a.statusCode = statusCode
	// TTFT falls back to the header time when no streaming first-token
	// was observed (non-streaming responses, or streams that errored
	// before emitting).
	if a.ttft == 0 && a.headerTime > 0 {
		a.ttft = a.headerTime
	}
	duration := time.Since(a.start)
	recordAttempt(a.channelID, a.ttft, duration, outcome == AttemptSuccess, statusCode, a.upstreamReqID)
}

// EndRequest finalizes the request-level metric and folds it into the
// global counters.
func (s *RequestSpan) EndRequest(status int) {
	class := classifyRequest(s, status)
	duration := time.Since(s.start)
	retries := len(s.attempts)
	if retries > 0 {
		retries-- // the first attempt is not a retry
	}
	recordRequest(s.meta, class, status, retries, duration)
}

// classifyRequest derives the request class from the captured status,
// retry trail and stream-interruption flag.
func classifyRequest(s *RequestSpan, status int) RequestClass {
	if s.streamGone {
		return ClassStreamInterrupted
	}
	switch {
	case status == 0:
		return ClassInternalError
	case status >= 200 && status < 300:
		return ClassSuccess
	case status == httpStatusServiceUnavailable && len(s.attempts) == 0:
		return ClassNoChannel
	case status >= 500:
		return ClassUpstreamError
	case status >= 400:
		return ClassClientError
	}
	return ClassInternalError
}

// httpStatusServiceUnavailable avoids importing net/http here just for
// one constant.
const httpStatusServiceUnavailable = 503

// ---------------------------------------------------------------------------
// Global registry: request counters + per-channel rolling windows
// ---------------------------------------------------------------------------

const rollingWindowSize = 100

var (
	// Request-level counters, one slot per class. Atomic so the hot
	// relay path never contends on a mutex.
	requestCounts [numClasses]atomic.Int64

	// Per-channel rolling stats. sync.Map keeps the read path lock-free
	// once a channel's entry exists.
	channelWindows sync.Map // channelID (int64) -> *rollingWindow
)

// classOrder fixes the iteration order for Snapshot so the JSON surface
// is stable.
var classOrder = []RequestClass{
	ClassSuccess, ClassClientError, ClassUpstreamError,
	ClassNoChannel, ClassStreamInterrupted, ClassInternalError,
}

// numClasses is the count of RequestClass values; it backs the
// requestCounts array (array lengths must be constant).
const numClasses = 6

type rollingWindow struct {
	mu      sync.Mutex
	samples [rollingWindowSize]attemptSample
	next    int
	filled  bool
}

type attemptSample struct {
	ttft       time.Duration
	duration   time.Duration
	success    bool
	statusCode int
	ts         time.Time
}

func recordRequest(meta RequestMeta, class RequestClass, status, retries int, duration time.Duration) {
	for i, c := range classOrder {
		if c == class {
			requestCounts[i].Add(1)
			break
		}
	}
}

func recordAttempt(channelID int64, ttft, duration time.Duration, success bool, statusCode int, upstreamReqID string) {
	raw, _ := channelWindows.LoadOrStore(channelID, &rollingWindow{})
	w := raw.(*rollingWindow)
	w.mu.Lock()
	w.samples[w.next] = attemptSample{
		ttft:       ttft,
		duration:   duration,
		success:    success,
		statusCode: statusCode,
		ts:         time.Now(),
	}
	w.next = (w.next + 1) % rollingWindowSize
	if w.next == 0 {
		w.filled = true
	}
	w.mu.Unlock()
}

// ChannelStat is the aggregated view of one channel's rolling window.
type ChannelStat struct {
	ChannelID    int64   `json:"channel_id"`
	Samples      int     `json:"samples"`
	SuccessRate  float64 `json:"success_rate"`
	AvgTTFTMs    float64 `json:"avg_ttft_ms"`     // average time to first token
	P95TTFTMs    float64 `json:"p95_ttft_ms"`     // 95th percentile TTFT
	AvgLatencyMs float64 `json:"avg_latency_ms"`  // average attempt duration
	LastTTFTMs   float64 `json:"last_ttft_ms"`    // most recent TTFT
	LastAttemptAt int64  `json:"last_attempt_at"` // unix seconds of the last sample
}

// RequestSnapshot is the request-level counter summary.
type RequestSnapshot struct {
	Class   string `json:"class"`
	Count   int64  `json:"count"`
}

// Snapshot is the full observability surface for the admin API. It is
// safe to call concurrently with the relay path.
type Snapshot struct {
	Requests      []RequestSnapshot `json:"requests"`
	ChannelStats  []ChannelStat     `json:"channel_stats"`
	Window        int               `json:"window"`
	GeneratedAt   int64             `json:"generated_at"`
}

// TakeSnapshot builds a point-in-time view of the registry.
func TakeSnapshot() Snapshot {
	reqs := make([]RequestSnapshot, 0, len(classOrder))
	for i, c := range classOrder {
		reqs = append(reqs, RequestSnapshot{
			Class: c.String(),
			Count: requestCounts[i].Load(),
		})
	}
	var stats []ChannelStat
	channelWindows.Range(func(key, val any) bool {
		id := key.(int64)
		w := val.(*rollingWindow)
		stats = append(stats, computeChannelStat(id, w))
		return true
	})
	return Snapshot{
		Requests:     reqs,
		ChannelStats: stats,
		Window:       rollingWindowSize,
		GeneratedAt:  time.Now().Unix(),
	}
}

// ChannelStatFor returns the aggregated stat for one channel, or nil when
// no attempt has been recorded for it yet.
func ChannelStatFor(channelID int64) *ChannelStat {
	raw, ok := channelWindows.Load(channelID)
	if !ok {
		return nil
	}
	w := raw.(*rollingWindow)
	s := computeChannelStat(channelID, w)
	return &s
}

func computeChannelStat(id int64, w *rollingWindow) ChannelStat {
	w.mu.Lock()
	n := w.next
	if w.filled {
		n = rollingWindowSize
	}
	if n == 0 {
		w.mu.Unlock()
		return ChannelStat{ChannelID: id}
	}
	samples := make([]attemptSample, n)
	copy(samples, w.samples[:n])
	last := samples[len(samples)-1]
	w.mu.Unlock()

	successes := 0
	var ttftSum, durSum time.Duration
	ttfts := make([]time.Duration, 0, n)
	for _, s := range samples {
		if s.success {
			successes++
		}
		ttftSum += s.ttft
		durSum += s.duration
		ttfts = append(ttfts, s.ttft)
	}
	return ChannelStat{
		ChannelID:     id,
		Samples:       n,
		SuccessRate:   float64(successes) / float64(n),
		AvgTTFTMs:     float64(ttftSum.Milliseconds()) / float64(n),
		P95TTFTMs:     float64(percentile(ttfts, 95).Milliseconds()),
		AvgLatencyMs:  float64(durSum.Milliseconds()) / float64(n),
		LastTTFTMs:    float64(last.ttft.Milliseconds()),
		LastAttemptAt: last.ts.Unix(),
	}
}

// percentile returns the p-th percentile of a duration slice (linear
// interpolation). The input is sorted in place.
func percentile(values []time.Duration, p float64) time.Duration {
	if len(values) == 0 {
		return 0
	}
	sortDurations(values)
	if len(values) == 1 {
		return values[0]
	}
	rank := p / 100 * float64(len(values)-1)
	lo := int(rank)
	hi := lo + 1
	if hi >= len(values) {
		return values[len(values)-1]
	}
	frac := rank - float64(lo)
	return time.Duration(float64(values[lo]) + frac*float64(values[hi]-values[lo]))
}

func sortDurations(a []time.Duration) {
	// Insertion sort: the rolling window is small (<=100) and this
	// avoids the sort import for a single call site.
	for i := 1; i < len(a); i++ {
		key := a[i]
		j := i - 1
		for j >= 0 && a[j] > key {
			a[j+1] = a[j]
			j--
		}
		a[j+1] = key
	}
}

// Reset clears all collected metrics. Tests use it to start from a clean
// slate; production never calls it.
func Reset() {
	for i := range classOrder {
		requestCounts[i].Store(0)
	}
	channelWindows.Range(func(key, _ any) bool {
		channelWindows.Delete(key)
		return true
	})
}

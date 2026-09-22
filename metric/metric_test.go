package metric

import (
	"testing"
	"time"
)

func TestRequestClassification(t *testing.T) {
	cases := []struct {
		name   string
		span   *RequestSpan
		status int
		want   RequestClass
	}{
		{"success", &RequestSpan{}, 200, ClassSuccess},
		{"client_error", &RequestSpan{}, 400, ClassClientError},
		{"upstream_error", &RequestSpan{attempts: []*AttemptSpan{{}}}, 502, ClassUpstreamError},
		{"no_channel", &RequestSpan{}, 503, ClassNoChannel},
		{"stream_interrupted", &RequestSpan{streamGone: true}, 200, ClassStreamInterrupted},
		{"internal", &RequestSpan{}, 0, ClassInternalError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := classifyRequest(c.span, c.status)
			if got != c.want {
				t.Fatalf("classifyRequest(%s, %d) = %s, want %s", c.name, c.status, got, c.want)
			}
		})
	}
}

func TestRequestSpanLifecycle(t *testing.T) {
	Reset()
	span := BeginRequest(RequestMeta{Model: "gpt-4o", Mode: "chat", Stream: true})

	a := span.BeginAttempt(101)
	time.Sleep(2 * time.Millisecond)
	a.RecordHeaders("req-upstream-1")
	time.Sleep(2 * time.Millisecond)
	a.RecordFirstToken() // streaming: refine TTFT to first token
	span.EndAttempt(a, AttemptSuccess, 200)

	b := span.BeginAttempt(102)
	time.Sleep(2 * time.Millisecond)
	b.RecordHeaders("req-upstream-2")
	span.EndAttempt(b, AttemptUpstreamError, 500)

	span.EndRequest(200)

	snap := TakeSnapshot()
	var successCount, upstreamCount int64
	for _, r := range snap.Requests {
		if r.Class == "success" {
			successCount = r.Count
		}
		if r.Class == "upstream_error" {
			upstreamCount = r.Count
		}
	}
	if successCount != 1 {
		t.Fatalf("success count = %d, want 1", successCount)
	}
	// The request itself succeeded, so upstream_error request count is 0.
	if upstreamCount != 0 {
		t.Fatalf("upstream request count = %d, want 0", upstreamCount)
	}

	// Channel 101: 1 success sample; channel 102: 1 failure sample.
	c101 := ChannelStatFor(101)
	if c101 == nil || c101.Samples != 1 || c101.SuccessRate != 1.0 {
		t.Fatalf("channel 101 stat = %+v", c101)
	}
	c102 := ChannelStatFor(102)
	if c102 == nil || c102.Samples != 1 || c102.SuccessRate != 0.0 {
		t.Fatalf("channel 102 stat = %+v", c102)
	}

	// TTFT must be recorded and positive for the streaming attempt.
	if c101.AvgTTFTMs <= 0 {
		t.Fatalf("channel 101 avg ttft = %v, want > 0", c101.AvgTTFTMs)
	}
	// Channel 102 failed before first token: TTFT falls back to header time.
	if c102.AvgTTFTMs <= 0 {
		t.Fatalf("channel 102 avg ttft = %v, want > 0 (header fallback)", c102.AvgTTFTMs)
	}
}

func TestRollingWindowCaps(t *testing.T) {
	Reset()
	span := BeginRequest(RequestMeta{Model: "m"})
	// Push more samples than the window size; the snapshot must reflect
	// only the last rollingWindowSize attempts.
	for i := 0; i < rollingWindowSize+20; i++ {
		a := span.BeginAttempt(1)
		a.RecordHeaders("")
		span.EndAttempt(a, AttemptSuccess, 200)
	}
	span.EndRequest(200)
	stat := ChannelStatFor(1)
	if stat == nil {
		t.Fatal("expected channel stat")
	}
	if stat.Samples != rollingWindowSize {
		t.Fatalf("samples = %d, want %d", stat.Samples, rollingWindowSize)
	}
	if stat.SuccessRate != 1.0 {
		t.Fatalf("success rate = %v, want 1.0", stat.SuccessRate)
	}
}

func TestPercentile(t *testing.T) {
	if got := percentile(nil, 95); got != 0 {
		t.Fatalf("percentile(nil) = %v, want 0", got)
	}
	ms := time.Millisecond
	values := []time.Duration{1 * ms, 2 * ms, 3 * ms, 4 * ms, 5 * ms, 6 * ms, 7 * ms, 8 * ms, 9 * ms, 10 * ms}
	p95 := percentile(values, 95)
	// 95th percentile of 1..10ms with linear interpolation: rank=8.55,
	// value = 9ms + 0.55*(10ms-9ms) = 9.55ms.
	want := 9*time.Millisecond + 550*time.Microsecond
	if p95 < want-time.Microsecond || p95 > want+time.Microsecond {
		t.Fatalf("p95 = %v, want ~%v", p95, want)
	}
}

func TestRecordFirstTokenIdempotent(t *testing.T) {
	Reset()
	a := (&RequestSpan{}).BeginAttempt(1)
	a.RecordHeaders("rid")
	first := time.Now()
	a.RecordFirstToken()
	a.RecordFirstToken() // must not overwrite
	if !a.hasFirstToken {
		t.Fatal("hasFirstToken should be true")
	}
	_ = first
}

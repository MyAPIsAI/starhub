package metric

import (
	"testing"
	"time"
)

func TestPlatformMetricsSeparateModelsAndExcludeClientErrors(t *testing.T) {
	Reset()
	t.Cleanup(Reset)
	for _, sample := range []struct {
		model   string
		outcome AttemptClass
		status  int
	}{{"a", AttemptSuccess, 200}, {"b", AttemptUpstreamError, 503}, {"a", AttemptClientError, 400}, {"a", AttemptUnsupported, 0}} {
		span := BeginRequest(RequestMeta{Model: sample.model})
		a := span.BeginAttempt(7)
		a.headerTime = 50 * time.Millisecond
		span.EndAttempt(a, sample.outcome, sample.status)
		span.EndAttempt(a, sample.outcome, sample.status)
	}
	rows := PlatformMetricRows(true, false, 0, time.Now().Unix())
	if len(rows) != 2 || rows[0].ModelName != "a" || rows[0].SuccessCount != 1 || rows[0].AttemptCount != 1 || rows[1].Status5xxCount != 1 {
		t.Fatalf("incorrect attempt classification: %+v", rows)
	}
	all := PlatformMetricRows(false, true, 0, time.Now().Unix())
	if len(all) != 1 || all[0].AttemptCount != 2 || all[0].SuccessRate != 50 || all[0].WindowEnd.Sub(all[0].WindowStart) != 15*time.Minute {
		t.Fatalf("incorrect window: %+v", all)
	}
}

func TestPlatformMetricsDoNotInventStreamingFirstToken(t *testing.T) {
	Reset()
	t.Cleanup(Reset)
	span := BeginRequest(RequestMeta{Model: "stream", Stream: true})
	a := span.BeginAttempt(1)
	a.headerTime = time.Millisecond
	span.EndAttempt(a, AttemptUpstreamError, 502)
	rows := PlatformMetricRows(true, false, 0, time.Now().Unix())
	if len(rows) != 1 || rows[0].TTFTCount != 0 || rows[0].FirstTokenMissingCount != 1 {
		t.Fatalf("headers mistaken for content: %+v", rows)
	}
}

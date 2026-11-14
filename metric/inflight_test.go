package metric

import "testing"

func TestInflightSpansCountOnceAcrossRetries(t *testing.T) {
	Reset()
	t.Cleanup(Reset)
	s := BeginRequest(RequestMeta{})
	a := s.BeginAttempt(1)
	if requests, attempts := InflightCounts(); requests != 1 || attempts != 1 {
		t.Fatal(requests, attempts)
	}
	s.EndAttempt(a, AttemptUpstreamError, 500)
	s.EndAttempt(a, AttemptUpstreamError, 500)
	b := s.BeginAttempt(2)
	if _, attempts := InflightCounts(); attempts != 1 {
		t.Fatal(attempts)
	}
	s.EndAttempt(b, AttemptSuccess, 200)
	s.EndRequest(200)
	s.EndRequest(200)
	if requests, attempts := InflightCounts(); requests != 0 || attempts != 0 {
		t.Fatal(requests, attempts)
	}
}

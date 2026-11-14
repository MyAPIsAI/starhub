package metric

import (
	"testing"
	"time"
)

func TestDurationHistogramQuantilesAndOverflow(t *testing.T) {
	var h DurationHistogram
	for i := 0; i < 99; i++ {
		h.Observe(80 * time.Microsecond)
	}
	h.Observe(20 * time.Second)
	s := h.Snapshot()
	if s.Count != 100 || s.P95US != 100 || s.P99US != 100 || s.MaxUS != 20000000 {
		t.Fatal(s)
	}
	var overflow DurationHistogram
	overflow.Observe(20 * time.Second)
	if overflow.Snapshot().P99US != 20000000 {
		t.Fatal(overflow.Snapshot())
	}
}

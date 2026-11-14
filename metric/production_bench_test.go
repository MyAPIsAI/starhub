package metric

import (
	"fmt"
	"testing"
	"time"
)

// One operation collects and seals 4096 attempts without exceeding admission
// limits. Rejected writes must never be counted as fast successful samples.
func BenchmarkProductionCollectSeal(b *testing.B) {
	for _, keys := range []int{1, 128, 4096} {
		b.Run(fmt.Sprintf("keys_%d", keys), func(b *testing.B) {
			Reset()
			b.Cleanup(Reset)
			meta := RequestMeta{Model: "benchmark", Stream: true}
			const attempts = 4096
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				now := time.Now().Truncate(time.Minute)
				StartProduction(now.Add(-time.Minute))
				for j := 0; j < attempts; j++ {
					a := &AttemptSpan{channelID: int64(j%keys + 1), start: now, endedAt: now, sent: true, ended: true, outcome: AttemptSuccess, hasFirstToken: true, ttft: time.Millisecond, route: AttemptMetadata{Generation: "g", Protocol: "OpenAI", Group: "default", Capability: "chat"}}
					recordProductionAttempt(meta, a, false)
				}
				batch := ClosedProductionMinutes(time.Now().Add(time.Minute))
				var collected int64
				for _, minute := range batch {
					collected += minute.TotalAttempts
					if minute.TotalAttempts > 0 && !minute.Complete {
						b.Fatal("benchmark lost coverage")
					}
				}
				if collected != attempts {
					b.Fatal("benchmark lost attempts")
				}
				AckProduction(batch)
			}
			b.StopTimer()
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*attempts), "ns/attempt")
		})
	}
}

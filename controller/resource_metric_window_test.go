package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"starhub/metric"
	"testing"
	"time"
)

func TestResourceModelMinutesMatchConsumerWindowAndKeepOpenMinuteUnknown(t *testing.T) {
	newTestDB(t)
	metric.Reset()
	t.Cleanup(metric.Reset)
	for _, minutes := range []int{1, 15, 30, 126} {
		response := call(ResourcePlatformMetrics, testRequest{target: fmt.Sprintf("/api/metric/model/redis?minutes=%d", minutes)})
		envelope := expectStatus(t, response, http.StatusOK)
		var data struct {
			Buckets []struct {
				Minute int64 `json:"minute"`
				Models []struct {
					Coverage string `json:"coverage_status"`
				} `json:"models"`
			} `json:"buckets"`
		}
		if err := json.Unmarshal(envelope.Data, &data); err != nil {
			t.Fatal(err)
		}
		if len(data.Buckets) != minutes {
			t.Fatalf("requested %d got %d", minutes, len(data.Buckets))
		}
		for i, b := range data.Buckets {
			if b.Minute%60 != 0 || i > 0 && b.Minute != data.Buckets[i-1].Minute+60 {
				t.Fatal("noncontiguous minute window")
			}
		}
		last := data.Buckets[len(data.Buckets)-1]
		if last.Minute > time.Now().Unix() || len(last.Models) != 1 || last.Models[0].Coverage != "minute_open" {
			t.Fatal("open minute became complete")
		}
	}
	for _, query := range []string{"minutes=0", "minutes=-1", "minutes=127", "minutes=bad", "minutes=", "minutes=1&minutes=2", "minutes=2&start_timestamp=1", "minutes=2&end_timestamp=2"} {
		expectStatus(t, call(ResourcePlatformMetrics, testRequest{target: "/api/metric/model/redis?" + query}), 400)
	}
}

package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"starhub/metric"
	"starhub/model"
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

func TestMetricReadsNeverSealOrWriteUnpublishedProduction(t *testing.T) {
	newTestDB(t)
	metric.Reset()
	t.Cleanup(metric.Reset)
	now := time.Now().Truncate(time.Minute)
	metric.StartProduction(now.Add(-time.Minute))
	s := metric.BeginRequest(metric.RequestMeta{Model: "read-only"})
	a := s.BeginAttempt(1)
	a.SetRouteMetadata(metric.AttemptMetadata{Generation: "fixture"})
	a.MarkSent()
	s.EndAttempt(a, metric.AttemptSuccess, 200)
	if err := model.DB.Exec("CREATE TRIGGER reject_metric_publication BEFORE INSERT ON production_telemetry_minutes BEGIN SELECT RAISE(ABORT, 'query must not publish'); END").Error; err != nil {
		t.Fatal(err)
	}
	for _, url := range []string{"/api/metric/model/redis?minutes=2", "/api/metric/attempts"} {
		expectStatus(t, call(ResourcePlatformMetrics, testRequest{target: url}), 200)
	}
	var count int64
	if err := model.DB.Model(&model.ProductionTelemetryMinute{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("query published a minute", err)
	}
	batch := metric.ClosedProductionMinutes(now.Add(time.Minute))
	if len(batch) != 2 || len(batch[1].Attempts) != 1 {
		t.Fatal("query consumed pending production")
	}
}

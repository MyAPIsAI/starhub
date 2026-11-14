package service

import (
	"encoding/json"
	"starhub/metric"
	"starhub/model"
	"testing"
	"time"
)

func TestProductionQuarterHistorySurvivesRestartAndReportsGaps(t *testing.T) {
	resourceTestDB(t)
	metric.Reset()
	t.Cleanup(metric.Reset)
	now := time.Now().Truncate(15 * time.Minute)
	start := now.Add(-15 * time.Minute).Unix()
	for i := 0; i < 15; i++ {
		at := start + int64(i)*60
		b := metric.ProductionMinute{Minute: at, BatchID: time.Unix(at, 0).String(), Complete: true}
		if i == 3 {
			b.Entries = []metric.ProductionEntry{{ChannelID: 7, ModelName: "persisted", Generation: "g", Counts: metric.ProductionCounts{N: 2, S: 2}}}
		}
		raw, _ := json.Marshal(b)
		if err := model.SaveProductionTelemetry([]model.ProductionTelemetryMinute{{Minute: at, BatchID: b.BatchID, Complete: true, Payload: string(raw)}}, now); err != nil {
			t.Fatal(err)
		}
	}
	metric.Reset()
	for _, byModel := range []bool{true, false} {
		rows, coverage, err := ProductionQuarterMetrics(byModel, start, now.Unix()-60, now)
		if err != nil || len(rows) != 1 || rows[0].TotalCount != 2 || len(coverage) != 1 || !coverage[0].Complete {
			t.Fatalf("durable quarter: rows=%+v coverage=%+v err=%v", rows, coverage, err)
		}
	}
	if err := model.DB.Delete(&model.ProductionTelemetryMinute{}, start+60).Error; err != nil {
		t.Fatal(err)
	}
	_, coverage, err := ProductionQuarterMetrics(true, start, now.Unix()-60, now)
	if err != nil || coverage[0].Complete || coverage[0].CompleteMinutes != 14 {
		t.Fatalf("missing minute asserted complete: %+v %v", coverage, err)
	}
	_, coverage, err = ProductionQuarterMetrics(true, start+120, now.Unix()-60, now)
	if err != nil || coverage[0].Complete || coverage[0].Requested != 13 {
		t.Fatalf("partial range asserted complete: %+v %v", coverage, err)
	}
}

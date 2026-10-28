package controller

import (
	"fmt"
	"testing"
	"time"

	"starhub/common"
	"starhub/model"
)

func TestServiceQualityPercentilePreservesDuplicateSampleWeights(t *testing.T) {
	newTestDB(t)
	for index, tc := range []struct {
		low, mid, high, unmeasured int
		want                       int64
	}{
		{94, 1, 5, 0, 10},
		{95, 0, 5, 0, 1},
		{1, 0, 99, 0, 100},
		{19, 1, 1, 0, 10},
		{1, 0, 0, 100, 1},
	} {
		name := fmt.Sprintf("frequency-%d", index)
		logs := make([]model.Log, 0, tc.low+tc.mid+tc.high+tc.unmeasured)
		for _, group := range []struct {
			count    int
			duration int64
		}{{tc.low, 1}, {tc.mid, 10}, {tc.high, 100}, {tc.unmeasured, 0}} {
			for n := 0; n < group.count; n++ {
				logs = append(logs, model.Log{UserID: 1, Model: name, DurationMs: group.duration, Status: 1, CreatedAt: time.Now().Unix()})
			}
		}
		if err := model.DB.CreateInBatches(logs, 100).Error; err != nil {
			t.Fatal(err)
		}
		rec := call(GetServiceQuality, testRequest{target: "/api/service-quality?hours=24&model=" + name, userID: 1, role: common.RoleAdmin})
		var got model.ServiceQualitySummary
		decodeData(t, rec, &got)
		if got.Overall.P95DurationMs != tc.want || len(got.ByModel) != 1 || got.ByModel[0].P95DurationMs != tc.want || len(got.ByDay) != 1 || got.ByDay[0].P95DurationMs != tc.want || got.Overall.TotalRequests != int64(len(logs)) {
			t.Fatalf("histogram weights changed nearest-rank percentile: got=%+v want=%d", got, tc.want)
		}
	}
}

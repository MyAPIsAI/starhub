package controller

import (
	"net/http"
	"testing"
	"time"

	"starhub/common"
	"starhub/metric"
	"starhub/model"
)

func TestMetricsPersistedUsage(t *testing.T) {
	newTestDB(t)
	metric.Reset()
	t.Cleanup(metric.Reset)
	read := func() model.RequestLogSummary {
		t.Helper()
		var data struct {
			metric.Snapshot
			Usage model.RequestLogSummary `json:"usage"`
		}
		decodeData(t, call(GetMetrics, testRequest{target: "/api/metrics", userID: 1, role: common.RoleAdmin}), &data)
		for _, request := range data.Requests {
			if request.Count != 0 {
				t.Fatalf("expected cleared process counters, got %+v", data.Requests)
			}
		}
		return data.Usage
	}
	if got := read(); got != (model.RequestLogSummary{}) {
		t.Fatalf("empty usage = %+v", got)
	}

	// Include old calls, multiple users, free successes, failures and cancellations.
	for i, status := range []int{1, 1, 1, 2, 3} {
		if err := model.InsertLog(&model.Log{
			UserID: int64(i + 1), Status: status,
			CreatedAt: time.Now().AddDate(-1, 0, 0).Unix(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	// Simulate a process restart: persisted totals must survive registry reset.
	metric.BeginRequest(metric.RequestMeta{}).EndRequest(http.StatusOK)
	metric.Reset()
	want := model.RequestLogSummary{TotalRequests: 5, SuccessCount: 3, FailedCount: 2}
	if got := read(); got != want {
		t.Fatalf("persisted usage = %+v, want %+v", got, want)
	}
}

func TestMetricsUsageDatabaseError(t *testing.T) {
	newTestDB(t)
	if err := model.DB.Migrator().DropTable(&model.Log{}); err != nil {
		t.Fatal(err)
	}
	expectError(t, call(GetMetrics, testRequest{target: "/api/metrics"}),
		http.StatusInternalServerError, "request statistics unavailable")
}

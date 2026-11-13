package service

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"starhub/model"
	"testing"
)

func TestUpstreamUsageKeepsLocalNanoAndLargeAggregateExact(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success":true,"data":{"total_used":%d,"total_available":0,"total_granted":0}}`, int64(math.MaxInt64))
	}))
	defer server.Close()
	ch := &model.Channel{Name: "nano", Key: "fixture-a\nfixture-b", BaseURL: server.URL, UsedQuotaRemainderNano: 1}
	result, err := FetchUpstreamTokenUsage(context.Background(), ch)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Supported {
		t.Fatalf("upstream fixture was rejected: %+v", result.Keys)
	}
	if result.LocalUsedQuotaNano != "1" || result.UpstreamTotal != math.MaxInt64 || result.UpstreamTotalNano != "36893488147419103228000" || result.DiffNano != "36893488147419103227999" {
		t.Fatalf("usage aggregation lost precision: %+v", result)
	}
}

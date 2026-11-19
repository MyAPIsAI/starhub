package model

import (
	"math"
	"testing"
	"time"

	"starhub/common"
	"starhub/setting"
)

func TestPublicStatusMetricsRealSamplesAndPrivateIsolation(t *testing.T) {
	newTestDB(t)
	setting.ResetAllToDefaults()
	t.Cleanup(setting.ResetAllToDefaults)
	ch := addChannel(t, "public", "default", "alpha,beta,unused", 0, 1, common.ChannelStatusEnabled)
	private := addChannel(t, "private", "private", "alpha", 0, 1, common.ChannelStatusEnabled)
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	view, scope, err := readPublicStatusCatalog(t.Context(), now)
	if err != nil {
		t.Fatal(err)
	}
	start, _ := time.Parse("2006-01-02", view.HistoryStart)
	for _, row := range []Log{
		{ChannelID: ch.ID, Model: "alpha", Status: 1, DurationMs: 100, CreatedAt: now.Add(-time.Hour).Unix()},
		{ChannelID: ch.ID, Model: "alpha", Status: 2, CreatedAt: now.Add(-2 * time.Hour).Unix()},
		{ChannelID: ch.ID, Model: "alpha", Status: 1, DurationMs: 300, CreatedAt: now.Add(-48 * time.Hour).Unix()},
		{ChannelID: ch.ID, Model: "beta", Status: 2, DurationMs: 200, CreatedAt: now.Unix()},
		{ChannelID: ch.ID, Model: "beta", Status: 2, DurationMs: 400, CreatedAt: now.Unix()},
		{ChannelID: ch.ID, Model: "alpha", Status: 3, DurationMs: 9999, CreatedAt: now.Unix()},
		{ChannelID: private.ID, Model: "alpha", Status: 1, CreatedAt: now.Unix()},
		{ChannelID: ch.ID, Model: "alpha", Status: 1, CreatedAt: view.WindowEnd},
		{ChannelID: ch.ID, Model: "alpha", Status: 1, CreatedAt: start.Unix() - 1},
	} {
		if err := DB.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := populatePublicStatusMetrics(t.Context(), view, scope); err != nil {
		t.Fatal(err)
	}
	alpha, beta, unused := view.Models[0], view.Models[1], view.Models[2]
	if alpha.Recent.TotalRequests != 2 || *alpha.Recent.SuccessRate != .5 || *alpha.Recent.AvgDurationMS != 100 || alpha.History.TotalRequests != 3 || math.Abs(*alpha.History.SuccessRate-2.0/3) > 1e-9 || *alpha.History.AvgDurationMS != 200 {
		t.Fatalf("alpha metrics: %+v", alpha)
	}
	if beta.Recent.TotalRequests != 2 || *beta.Recent.SuccessRate != 0 || beta.State != "degraded" || unused.Recent.SuccessRate != nil || unused.Recent.AvgDurationMS != nil || unused.State != "unknown" || view.State != "degraded" {
		t.Fatalf("states: %+v", view)
	}
	if len(alpha.Days) != 90 || alpha.Days[0].Date != view.HistoryStart || alpha.Days[0].SuccessRate != nil || alpha.Days[89].Date != "2026-11-19" || *alpha.Days[89].SuccessRate != .5 {
		t.Fatal("history is not actual UTC daily samples")
	}
}

func TestPublicStatusMetricsWindowBoundaryAndReadFailure(t *testing.T) {
	newTestDB(t)
	setting.ResetAllToDefaults()
	t.Cleanup(setting.ResetAllToDefaults)
	ch := addChannel(t, "boundary", "default", "alpha", 0, 1, 1)
	now := time.Date(2026, 11, 19, 0, 0, 0, 0, time.FixedZone("offset", 4*3600))
	view, scope, err := readPublicStatusCatalog(t.Context(), now)
	if err != nil {
		t.Fatal(err)
	}
	for _, at := range []int64{view.WindowStart - 1, view.WindowStart} {
		if err := DB.Create(&Log{ChannelID: ch.ID, Model: "alpha", Status: 1, CreatedAt: at}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := populatePublicStatusMetrics(t.Context(), view, scope); err != nil {
		t.Fatal(err)
	}
	if view.Models[0].Recent.TotalRequests != 1 || view.Models[0].History.TotalRequests != 2 || view.Models[0].Days[89].Date != "2026-11-18" || view.State != "operational" {
		t.Fatal(view)
	}
	if err := DB.Migrator().DropTable(&Log{}); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPublicStatusOverview(t.Context()); err == nil {
		t.Fatal("missing logs silently produced healthy state")
	}
}

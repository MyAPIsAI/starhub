package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestOwnedTokenUsageUsesRegionalAdmissionDaysAndExactKeyScope(t *testing.T) {
	newTestDB(t)
	u, token := periodicBillingToken(t, 1000)
	if err := DB.Create(&RegionProfile{ID: "uae", Name: "UAE", Timezone: "Asia/Dubai", Enabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := UpdateTokenFields(token.ID, map[string]any{"quota_region": "uae"}); err != nil {
		t.Fatal(err)
	}
	stamp := func(raw string) int64 {
		at, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			t.Fatal(err)
		}
		return at.Unix()
	}
	for _, entry := range []Log{
		{UserID: u.ID, TokenID: token.ID, Model: "first", Quota: 7, AdmittedAt: stamp("2026-11-13T19:59:00Z"), CreatedAt: stamp("2026-11-14T02:00:00Z"), Status: 1},
		{UserID: u.ID, TokenID: token.ID, Model: "second", Quota: 11, AdmittedAt: stamp("2026-11-13T20:00:00Z"), CreatedAt: stamp("2026-11-14T02:00:00Z"), Status: 2},
		{UserID: u.ID, TokenID: token.ID, Model: "first", Quota: 13, CreatedAt: stamp("2026-11-13T20:01:00Z"), Status: 1},
		{UserID: u.ID, TokenID: token.ID + 100, Model: "other-key", Quota: 9999, CreatedAt: stamp("2026-11-14T02:00:00Z"), Status: 1},
		{UserID: u.ID + 100, TokenID: token.ID, Model: "other-owner", Quota: 8888, CreatedAt: stamp("2026-11-14T02:00:00Z"), Status: 1},
	} {
		copy := entry
		if err := InsertLog(&copy); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Unix(stamp("2026-11-14T03:00:00Z"), 0)
	view, err := ReadOwnedTokenUsage(u.ID, token.ID, 1, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Trend) != 30 || view.Trend[28].Date != "2026-11-13" || view.Trend[28].Quota != 7 || view.Trend[29].Date != "2026-11-14" || view.Trend[29].Quota != 24 || view.Trend[29].Requests != 2 || view.Trend[29].FailedCount != 1 {
		t.Fatalf("regional trend: %+v", view.Trend)
	}
	if view.ModelTotal != 2 || len(view.Models) != 2 || view.Models[0].Model != "first" || view.Models[0].Quota != 20 {
		t.Fatalf("model scope: %+v", view.Models)
	}
	raw, _ := json.Marshal(view)
	if strings.Contains(string(raw), token.PlaintextKey()) || strings.Contains(string(raw), "key_hash") || strings.Contains(string(raw), "other-owner") {
		t.Fatal("usage view leaked credentials or foreign activity")
	}
	if _, err := ReadOwnedTokenUsage(u.ID+1, token.ID, 1, 1, now); !errors.Is(err, ErrTokenNotFound) {
		t.Fatal("usage ownership was bypassed")
	}
}

func TestOwnedTokenUsageSeparatesHistoryHoldsAndLifetimeTotals(t *testing.T) {
	newTestDB(t)
	u, token := periodicBillingToken(t, 1000)
	if err := ReserveQuota(u.ID, token.ID, false, 80, "history-spend"); err != nil {
		t.Fatal(err)
	}
	if err := SettleQuota(u.ID, token.ID, false, -30, 50, &Log{Model: "history", QuotaReservationID: "history-spend"}); err != nil {
		t.Fatal(err)
	}
	if err := ReserveQuota(u.ID, token.ID, false, 100, "history-held"); err != nil {
		t.Fatal(err)
	}
	view, err := ReadOwnedTokenUsage(u.ID, token.ID, 1, 1, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	c := view.Current
	if c.UsedQuota != 50 || c.ReservedQuota != 100 || c.LifetimeUsedQuota != 50 || c.QuotaLimit == nil || *c.QuotaLimit != 1000 || c.RemainingQuota == nil || *c.RemainingQuota != 850 {
		t.Fatalf("quota details: %+v", c)
	}
	if err := RefreshTokenQuota(token, time.Unix(token.PeriodEnd+1, 0)); err != nil {
		t.Fatal(err)
	}
	view, err = ReadOwnedTokenUsage(u.ID, token.ID, 1, 1, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if view.Current.UsedQuota != 0 || view.Current.LifetimeUsedQuota != 50 || view.PeriodTotal != 2 || view.Periods[1].UsedQuota != 50 || view.Periods[1].ReservedQuota != 100 {
		t.Fatalf("history after rollover: %+v", view)
	}
}

func TestOwnedTokenUsagePaginatesPeriodsAndModels(t *testing.T) {
	newTestDB(t)
	u, token := periodicBillingToken(t, 1000)
	now := time.Now()
	for i := 0; i < 24; i++ {
		now = time.Unix(token.PeriodEnd+1, 0)
		if err := RefreshTokenQuota(token, now); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 25; i++ {
		if err := InsertLog(&Log{UserID: u.ID, TokenID: token.ID, Model: fmt.Sprintf("model-%02d", i), Quota: int64(i), CreatedAt: now.Unix(), Status: 1}); err != nil {
			t.Fatal(err)
		}
	}
	view, err := ReadOwnedTokenUsage(u.ID, token.ID, 2, 2, now)
	if err != nil {
		t.Fatal(err)
	}
	if view.PeriodTotal != 25 || len(view.Periods) != 5 || view.ModelTotal != 25 || len(view.Models) != 5 || view.Models[0].Quota != 4 {
		t.Fatalf("usage pagination: %+v", view)
	}
	for _, day := range view.Trend[:29] {
		if day.Requests != 0 {
			t.Fatal("empty day was not an explicit zero")
		}
	}
}

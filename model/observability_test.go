package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"starhub/observability"
	"sync"
	"testing"
	"time"
)

func TestNFR09WindowSeverityDeliveryAndRecovery(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "observability.db")
	if err := InitDB(dbPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	ctx := context.Background()
	c := observability.Config{Timezone: "UTC", WorkStart: 9, WorkEnd: 18, ModelSuccess: .9, PaymentFailure: .05, MinimumSamples: 20}
	for i := 0; i < 100; i++ {
		status := 200
		if i < 6 {
			status = 503
		}
		row := TraceRequest{ID: observability.NewID(), Model: "fixture", Vendor: "fixture", StartedAt: now.Add(-time.Minute).UnixMilli(), EndedAt: now.Add(-time.Second).UnixMilli(), Status: status}
		if err := DB.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	old := TraceRequest{ID: observability.NewID(), EndedAt: now.Add(-6 * time.Minute).UnixMilli(), Status: 500}
	DB.Create(&old)
	client := TraceRequest{ID: observability.NewID(), EndedAt: now.Add(-time.Second).UnixMilli(), Status: 400}
	DB.Create(&client)
	if err := EvaluateOperationalAlerts(ctx, c, now); err != nil {
		t.Fatal(err)
	}
	var alert OperationalAlert
	if err := DB.Where("rule = ?", "platform_success").First(&alert).Error; err != nil || !alert.Active || alert.Severity != "critical" || alert.Value != .94 {
		t.Fatalf("bad platform window: %+v %v", alert, err)
	}
	if err := EvaluateOperationalAlerts(ctx, c, now); err != nil {
		t.Fatal(err)
	}
	var count int64
	DB.Model(&AlertDelivery{}).Count(&count)
	if count != 2 {
		t.Fatalf("duplicate or missing critical routes: %d", count)
	}
	var mu sync.Mutex
	received := map[string]int{}
	phoneIDs := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if payload["severity"] != "critical" || r.Header.Get("Idempotency-Key") == "" {
			t.Error("invalid notification")
		}
		received[r.URL.Path]++
		if r.URL.Path == "/phone" {
			phoneIDs = append(phoneIDs, r.Header.Get("Idempotency-Key"))
			if received[r.URL.Path] == 1 {
				w.WriteHeader(503)
				return
			}
		}
		w.WriteHeader(204)
	}))
	defer server.Close()
	c.PhoneURL, c.IMURL = server.URL+"/phone", server.URL+"/im"
	if err := DeliverOperationalAlerts(ctx, c, now); err != nil {
		t.Fatal(err)
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(dbPath); err != nil {
		t.Fatal(err)
	}
	if err := DeliverOperationalAlerts(ctx, c, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(phoneIDs) != 2 || phoneIDs[0] != phoneIDs[1] || received["/im"] != 1 {
		t.Fatalf("delivery retry/dedup failed: %+v", received)
	}
	DB.Model(&TraceRequest{}).Where("status = ?", 503).Update("status", 200)
	if err := EvaluateOperationalAlerts(ctx, c, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	DB.First(&alert, "id = ?", alert.ID)
	if alert.Active || alert.ResolvedAt == 0 || alert.Generation != 2 {
		t.Fatalf("recovery missing: %+v", alert)
	}
}

func TestNFR09SnapshotReconciliationAndWorkingHours(t *testing.T) {
	u, _, now := reconciliationFixture(t)
	ctx := context.Background()
	snapshot := resourceFixture()
	snapshot.PublishedAt = now.Add(-181 * time.Second).UTC().Format(time.RFC3339)
	if _, err := SaveResourceSnapshot(ctx, 0, snapshot); err != nil {
		t.Fatal(err)
	}
	DB.Model(&User{}).Where("id = ?", u.ID).Update("quota_nano_remainder", 1)
	reconciliationReport(t, u, now.AddDate(0, 0, -1), now)
	c := observability.Config{Timezone: "Asia/Dubai", WorkStart: 9, WorkEnd: 18, ModelSuccess: .95, PaymentFailure: .05, MinimumSamples: 20}
	if err := EvaluateOperationalAlerts(ctx, c, now); err != nil {
		t.Fatal(err)
	}
	var count int64
	DB.Model(&OperationalAlert{}).Where("active = ? AND severity = ?", true, "serious").Count(&count)
	if count != 2 {
		t.Fatalf("expected snapshot and reconciliation, got %d", count)
	}
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++; w.WriteHeader(204) }))
	defer server.Close()
	c.GeneralURL = server.URL
	if err := DeliverOperationalAlerts(ctx, c, now.Add(12*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if hits != 0 {
		t.Fatal("serious notification bypassed working hours")
	}
	if err := DeliverOperationalAlerts(ctx, c, now); err != nil {
		t.Fatal(err)
	}
	if hits != 2 {
		t.Fatalf("missing serious notifications: %d", hits)
	}
	DB.Model(&ReconciliationAlert{}).Where("delivered_at = 0").Count(&count)
	if count != 0 {
		t.Fatal("legacy financial notice was left pending")
	}
}

func TestNFR09RulesAndUnknownSamples(t *testing.T) {
	newTestDB(t)
	now := time.Now()
	ctx := context.Background()
	c := observability.Config{Timezone: "UTC", ModelSuccess: .95, PaymentFailure: .05, MinimumSamples: 2}
	for _, kind := range []string{"ledger", "settlement", "payment_callback"} {
		for i := 0; i < 2; i++ {
			if err := RecordObservation(kind, "fixture", false, now); err != nil {
				t.Fatal(err)
			}
		}
	}
	usage := 81.0
	DB.Create(&ObservabilityPulse{ID: "online", At: now.Unix(), Memory: &usage})
	DB.Create(&ObservabilityPulse{ID: "offline", At: now.Add(-time.Minute).Unix()})
	if err := EvaluateOperationalAlerts(ctx, c, now); err != nil {
		t.Fatal(err)
	}
	for rule, severity := range map[string]string{"ledger_failure": "critical", "settlement_failure": "critical", "payment_failure": "serious", "resource_usage": "general", "gateway_unavailable": "critical"} {
		var row OperationalAlert
		if err := DB.Where("rule = ? AND active = ?", rule, true).First(&row).Error; err != nil || row.Severity != severity {
			t.Fatalf("rule %s: %+v %v", rule, row, err)
		}
	}
	var count int64
	DB.Model(&OperationalAlert{}).Where("rule = ? AND active = ?", "platform_success", true).Count(&count)
	if count != 0 {
		t.Fatal("no traffic produced a success-rate alarm")
	}
}

func TestNFR09LegacyQueueAndOrderedRecovery(t *testing.T) {
	newTestDB(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	legacy := ReconciliationAlert{UserID: 42, Day: "2026-11-08", CreatedAt: now.Unix()}
	if err := DB.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := ImportLegacyReconciliationAlerts(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var count int64
	DB.Model(&AlertDelivery{}).Count(&count)
	if count != 1 {
		t.Fatal("legacy import not idempotent", count)
	}
	if err := AlertCondition(ctx, "reconciliation_difference", "42:2026-11-08", "serious", false, 0, 0, now); err != nil {
		t.Fatal(err)
	}
	received := []string{}
	fail := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		received = append(received, payload["transition"].(string))
		if fail {
			w.WriteHeader(503)
		} else {
			w.WriteHeader(204)
		}
	}))
	defer server.Close()
	c := observability.Config{GeneralURL: server.URL, Timezone: "UTC", WorkStart: 9, WorkEnd: 18}
	if err := DeliverOperationalAlerts(ctx, c, now); err != nil {
		t.Fatal(err)
	}
	if len(received) != 1 || received[0] != "firing" {
		t.Fatal("recovery overtook failed firing", received)
	}
	fail = false
	if err := DeliverOperationalAlerts(ctx, c, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(received) != 3 || received[1] != "firing" || received[2] != "resolved" {
		t.Fatal("out of order", received)
	}
}

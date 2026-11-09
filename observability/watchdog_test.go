package observability

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestWatchdogDetectsGatewayAndDatabaseOutsideBusinessHours(t *testing.T) {
	database := true
	gateway := true
	health := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-token" {
			t.Error("missing observer authentication")
		}
		_ = json.NewEncoder(w).Encode(map[string]bool{"gateway": gateway, "database": database})
	}))
	defer health.Close()
	events := []map[string]any{}
	notifier := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var event map[string]any
		_ = json.NewDecoder(r.Body).Decode(&event)
		events = append(events, event)
		w.WriteHeader(204)
	}))
	defer notifier.Close()
	c := Config{ObserverToken: "fixture-token", PhoneURL: notifier.URL, IMURL: notifier.URL}
	state := filepath.Join(t.TempDir(), "state.json")
	now := time.Date(2026, 11, 7, 23, 0, 0, 0, time.UTC)
	run := func() {
		t.Helper()
		if err := WatchOnce(context.Background(), c, health.URL, state, now); err != nil {
			t.Fatal(err)
		}
		now = now.Add(time.Minute)
	}
	run()
	if len(events) != 0 {
		t.Fatal("healthy gateway alerted")
	}
	database = false
	run()
	run()
	if len(events) != 2 || events[0]["rule"] != "database_failure" {
		t.Fatalf("database alert missing or duplicated: %+v", events)
	}
	gateway = false
	run()
	if len(events) != 4 || events[2]["rule"] != "gateway_unavailable" || events[2]["transition"] != "firing" {
		t.Fatal("unknown database health emitted a false recovery")
	}
	gateway = true
	run()
	if len(events) != 6 || events[4]["rule"] != "gateway_unavailable" || events[4]["transition"] != "resolved" {
		t.Fatal("gateway recovery missing")
	}
	database = true
	run()
	if len(events) != 8 || events[6]["transition"] != "resolved" {
		t.Fatal("database recovery missing")
	}
	health.Close()
	run()
	if len(events) != 10 || events[8]["rule"] != "gateway_unavailable" {
		t.Fatal("gateway downtime did not notify both channels")
	}
}

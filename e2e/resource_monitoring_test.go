package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"starhub/config"
	"starhub/model"
)

func TestADMAC09TechnicalConfirmationVersionAndAudit(t *testing.T) {
	handler, root := staffFixture(t)
	root.enroll("root", "Root-temporary-123")
	root.call("POST", "/api/admin/staff", map[string]any{"username": "sync-tech", "temporary_password": "Temporary-sync-123", "role": 20, "reason": "Synchronization acceptance"}, 200)
	tech := &staffHTTP{t: t, handler: handler, ip: "127.0.0.1:18490"}
	tech.enroll("sync-tech", "Temporary-sync-123")
	snapshot := model.ResourceSnapshot{ID: "full", Sequence: 17, PublishedAt: time.Now().UTC().Format(time.RFC3339), Channels: []model.ResourceChannel{}, Instances: []model.ResourceInstance{}}
	var calls atomic.Int32
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"version": 1042, "checksum": model.ResourceSnapshotChecksum(snapshot), "snapshot": snapshot})
	}))
	defer source.Close()
	if err := config.ConfigureSnapshotSource(source.URL, "synthetic-secret"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = config.ConfigureSnapshotSource("", "") })
	tech.call("POST", "/api/admin/resource-sync", map[string]bool{"confirm": false}, 400)
	if calls.Load() != 0 {
		t.Fatal("unconfirmed operation fetched source")
	}
	for range 2 {
		data := tech.call("POST", "/api/admin/resource-sync", map[string]bool{"confirm": true}, 200)
		if data["applied_version"] != float64(1042) || data["remote_version"] != float64(1042) || data["sequence"] != float64(17) || data["version_gap"] != float64(0) {
			t.Fatalf("wrong version: %+v", data)
		}
	}
	var audits []model.AuditLog
	if err := model.DB.Where("action = ?", "resource_sync").Find(&audits).Error; err != nil {
		t.Fatal(err)
	}
	if len(audits) != 2 || audits[0].ActorRole != 20 || audits[0].Username != "sync-tech" || audits[0].IP == "" {
		t.Fatalf("missing audited technical actor: %+v", audits)
	}
	raw, _ := json.Marshal(audits)
	if strings.Contains(string(raw), "synthetic-secret") || strings.Contains(string(raw), source.URL) {
		t.Fatal("source secret in audit")
	}
}

func TestADMAC14To16SnapshotMismatchAndActualIncidentReports(t *testing.T) {
	handler, root := staffFixture(t)
	root.enroll("root", "Root-temporary-123")
	root.call("POST", "/api/admin/staff", map[string]any{"username": "monitor-tech", "temporary_password": "Temporary-monitor-123", "role": 20, "reason": "Monitoring acceptance"}, 200)
	tech := &staffHTTP{t: t, handler: handler, ip: "127.0.0.1:18510"}
	tech.enroll("monitor-tech", "Temporary-monitor-123")
	now := time.Now().Truncate(time.Second)
	p := 4
	snapshot := model.ResourceSnapshot{ID: "monitoring", Sequence: 300, PublishedAt: now.Add(-190 * time.Second).Format(time.RFC3339), Channels: []model.ResourceChannel{{ID: "supplier", Name: "Fixture", Provider: "fixture", Enabled: true}}, Instances: []model.ResourceInstance{{ID: "resource-1", ChannelID: "supplier", UpstreamModel: "alpha", CanonicalModel: "alpha", Protocols: []string{"OpenAI"}, Capabilities: []string{"text"}, Status: "available", PriceTier: &p, QualityLevel: "S0", State: "hot"}}}
	if _, err := model.SaveResourceSnapshot(t.Context(), 0, snapshot); err != nil {
		t.Fatal(err)
	}
	data := tech.call("GET", "/api/admin/resource-sync", nil, 200)
	state := data["snapshot"].(map[string]any)
	if state["status"] != "expired" || state["sequence"] != float64(300) || state["age_seconds"].(float64) < 190 || state["published_at"] == nil {
		t.Fatalf("AC-14: %+v", state)
	}
	var stage atomic.Int32
	var reportCount atomic.Int32
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer synthetic-monitor-token" {
			t.Error("missing source authentication")
			w.WriteHeader(401)
			return
		}
		switch r.URL.Path {
		case "/events":
			after, _ := strconv.ParseInt(r.URL.Query().Get("after_version"), 10, 64)
			batch := model.ResourcePriceEventBatch{BaseVersion: after, Events: []model.ResourcePriceEvent{}}
			for version := after + 1; version <= 2; version++ {
				event := model.ResourcePriceEvent{EventID: fmt.Sprintf("event-%d", version), Version: version, Type: "resource.upsert", OccurredAt: now.Format(time.RFC3339)}
				price := int(4 + version*2)
				event.Resource.ResourceID, event.Resource.PriceTier = "resource-1", &price
				batch.Events = append(batch.Events, event)
			}
			_ = json.NewEncoder(w).Encode(batch)
		case "/snapshot":
			health := snapshot
			if stage.Load() >= 1 {
				health.Sequence++
				health.PublishedAt = now.Format(time.RFC3339)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"directory_version": 0, "snapshot": health})
		case "/incidents":
			var event map[string]any
			_ = json.NewDecoder(r.Body).Decode(&event)
			if event["gateway_id"] != "gw-2" || event["event_id"] != r.Header.Get("Idempotency-Key") {
				t.Error("AC-16: wrong gateway or unstable event identity")
			}
			reportCount.Add(1)
			w.WriteHeader(204)
		default:
			w.WriteHeader(404)
		}
	}))
	defer source.Close()
	if err := config.ConfigureSnapshotSource("", "synthetic-monitor-token"); err != nil {
		t.Fatal(err)
	}
	if err := config.ConfigureResourceMonitoring(source.URL+"/events", source.URL+"/snapshot", source.URL+"/incidents", "", "", ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = config.ConfigureResourceMonitoring("", "", "", "", "", "")
		_ = config.ConfigureSnapshotSource("", "")
	})
	if err := model.PollResourceMonitoring(t.Context(), now); err != nil {
		t.Fatal(err)
	}
	data = tech.call("GET", "/api/admin/resource-monitoring", nil, 200)
	if data["mismatch_total"] != float64(2) {
		t.Fatalf("AC-15: a single event batch must retain both conflicts: %+v", data)
	}
	stage.Store(1)
	if err := model.PollResourceMonitoring(t.Context(), now.Add(5*time.Second)); err != nil {
		t.Fatal(err)
	}
	data = tech.call("GET", "/api/admin/resource-monitoring", nil, 200)
	if data["mismatch_total"] != float64(2) {
		t.Fatalf("AC-15: %+v", data)
	}
	for _, value := range data["mismatches"].([]any) {
		row := value.(map[string]any)
		if row["resource_id"] != "resource-1" || row["snapshot_price"] != float64(4) || row["event_price"] == nil || row["at"] == nil {
			t.Fatal("AC-15: incomplete mismatch evidence")
		}
	}
	if err := model.PollResourceMonitoring(t.Context(), now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	data = tech.call("GET", "/api/admin/resource-sync", nil, 200)
	state = data["snapshot"].(map[string]any)
	if state["status"] != "fresh" || state["sequence"] != float64(301) {
		t.Fatalf("AC-14: no recovery: %+v", state)
	}
	data = tech.call("GET", "/api/admin/resource-monitoring", nil, 200)
	if data["mismatch_total"] != float64(2) {
		t.Fatal("an ongoing mismatch was duplicated by a newer snapshot")
	}
	runtime := model.NewResourceRuntime("gw-2")
	resource := snapshot.Instances[0]
	probeTime := now.Add(-30 * time.Minute)
	for range 5 {
		runtime.Record(resource, 1, probeTime, "failure", 0)
	}
	for range 5 {
		probeTime = probeTime.Add(5 * time.Minute)
		if len(runtime.DueProbes(probeTime)) != 1 {
			t.Fatal("half-open probe not due")
		}
		runtime.FinishProbe(resource, probeTime, false)
	}
	runtime.FinishProbe(resource, now, true)
	for range 7 {
		runtime.Record(resource, 1, now, "success", 5000)
	}
	if err := runtime.Publish(t.Context(), now); err != nil {
		t.Fatal(err)
	}
	data = tech.call("GET", "/api/admin/resource-monitoring", nil, 200)
	if data["incident_total"] != float64(2) {
		t.Fatalf("AC-16: missing or duplicated incidents: %+v", data)
	}
	for _, value := range data["incidents"].([]any) {
		if value.(map[string]any)["delivery_status"] != "pending" {
			t.Fatal("unsent event marked reported")
		}
	}
	if err := model.DeliverResourceIncidents(t.Context(), now); err != nil {
		t.Fatal(err)
	}
	data = tech.call("GET", "/api/admin/resource-monitoring", nil, 200)
	seen := map[string]bool{}
	for _, value := range data["incidents"].([]any) {
		row := value.(map[string]any)
		seen[row["type"].(string)] = true
		if row["gateway_id"] != "gw-2" || row["resource_id"] != "resource-1" || row["occurred_at"] == nil || row["delivery_status"] != "delivered" {
			t.Fatalf("AC-16: %+v", row)
		}
	}
	if !seen["half_open_failed"] || !seen["ewma_breach_twice"] || reportCount.Load() != 2 {
		t.Fatal("AC-16: both event types must reach the source")
	}
	raw, _ := json.Marshal(data)
	if strings.Contains(string(raw), "synthetic-monitor-token") || strings.Contains(string(raw), source.URL) {
		t.Fatal("monitoring exposed source credentials")
	}
	tech.call("GET", "/api/admin/resource-monitoring?incident_page=0", nil, 400)
	data = tech.call("GET", "/api/admin/resource-monitoring?mismatch_page=2&incident_page=2", nil, 200)
	if len(data["mismatches"].([]any)) != 0 || len(data["incidents"].([]any)) != 0 || data["incident_total"] != float64(2) {
		t.Fatal("history pagination changed totals")
	}
}

func TestADMAC10ActualGatewayFailuresAreInstanceLocal(t *testing.T) {
	env := setupRelayApp(t)
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(503)
		fmt.Fprint(w, `{"error":{"message":"synthetic unavailable"}}`)
	}))
	defer upstream.Close()
	p, score := 4, float64(90)
	snapshot := model.ResourceSnapshot{ID: "monitor", Sequence: 1, PublishedAt: time.Now().UTC().Format(time.RFC3339), Channels: []model.ResourceChannel{{ID: "source", Name: "Synthetic supplier", Provider: "fixture", Enabled: true}}, Instances: []model.ResourceInstance{{ID: "resource", ChannelID: "source", UpstreamModel: "alpha", CanonicalModel: "alpha", Protocols: []string{"OpenAI"}, Capabilities: []string{"text"}, ContextLength: 128000, Status: "available", PriceTier: &p, QualityLevel: "S0", State: "hot", Trend: "stable", Score: &score}}}
	if _, err := model.SaveResourceSnapshot(t.Context(), 0, snapshot); err != nil {
		t.Fatal(err)
	}
	name := "alpha"
	if _, err := model.SaveResourcePlacement(t.Context(), 1, "resource", model.ResourcePlacement{ModelID: &name, GroupIDs: []string{"default"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	ch := &model.Channel{Name: "monitor", Type: 1, Key: "synthetic-key", BaseURL: upstream.URL, Models: "alpha", Group: "default", Status: 1}
	if err := model.InsertChannel(ch); err != nil {
		t.Fatal(err)
	}
	if err := model.DB.Create(&model.ResourceGatewayManagedChannel{ChannelID: ch.ID, SourceChannelID: "source", Active: true, Profile: model.ResourceGatewayTextProfile}).Error; err != nil {
		t.Fatal(err)
	}
	if err := model.DB.Create(&model.ResourceGatewayRoute{ChannelID: ch.ID, Model: "alpha", Group: "default", Capabilities: `["text"]`, ContextLength: 128000}).Error; err != nil {
		t.Fatal(err)
	}
	for range 6 {
		res := doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, map[string]any{"model": "alpha", "messages": []map[string]string{{"role": "user", "content": "fixture"}}, "max_tokens": 1})
		if res.Status < 400 {
			t.Fatalf("failed upstream succeeded: %s", res.Body)
		}
	}
	if calls.Load() != 5 {
		t.Fatalf("open circuit continued sending: %d", calls.Load())
	}
	now := time.Now()
	if err := model.LocalResources.Publish(t.Context(), now); err != nil {
		t.Fatal(err)
	}
	other := model.NewResourceRuntime("second-gateway")
	if err := other.Publish(t.Context(), now); err != nil {
		t.Fatal(err)
	}
	rows, err := model.ReadAdminResourceStatus(t.Context(), now)
	if err != nil || len(rows) != 1 || rows[0].Calls != 5 {
		t.Fatalf("attempt attribution: %+v %v", rows, err)
	}
	states := map[string]string{}
	for _, g := range rows[0].Gateways {
		states[g.GatewayID] = g.State
	}
	if states[model.LocalResources.ID] != "open" || states[other.ID] != "normal" {
		t.Fatalf("state leaked across gateways: %+v", states)
	}
	raw, _ := json.Marshal(rows)
	if strings.Contains(string(raw), "synthetic-key") || strings.Contains(string(raw), upstream.URL) {
		t.Fatal("credential leaked to resource monitor")
	}
}

func TestResourceMonitorSearchAndRelatedCalls(t *testing.T) {
	_, root := staffFixture(t)
	root.enroll("root", "Root-temporary-123")
	p := 4
	snapshot := model.ResourceSnapshot{ID: "search", Sequence: 1, PublishedAt: time.Now().UTC().Format(time.RFC3339), Channels: []model.ResourceChannel{{ID: "supplier", Name: "Fixture supplier", Provider: "fixture", Enabled: true}}}
	for i := 1; i <= 25; i++ {
		snapshot.Instances = append(snapshot.Instances, model.ResourceInstance{ID: fmt.Sprintf("resource-%02d", i), ChannelID: "supplier", UpstreamModel: "Alpha", CanonicalModel: "Alpha", Protocols: []string{"OpenAI"}, Capabilities: []string{"text"}, ContextLength: 128000, Status: "available", PriceTier: &p, QualityLevel: "S0", State: "hot"})
	}
	if _, err := model.SaveResourceSnapshot(t.Context(), 0, snapshot); err != nil {
		t.Fatal(err)
	}
	missingReports := root.call("GET", "/api/admin/resource-instances?status=unknown", nil, 200)
	if missingReports["total"] != float64(25) {
		t.Fatal("resources without gateway reports must show unknown")
	}
	for range 5 {
		model.LocalResources.Record(snapshot.Instances[20], 1, time.Now(), "failure", 0)
	}
	if err := model.LocalResources.Publish(t.Context(), time.Now()); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		query        string
		total, count int
	}{
		{"q=%20RESOURCE-21%20", 1, 1},
		{"q=alpha&page=2", 25, 5},
		{"q=resource-2&vendor=fixture&model=Alpha&tier=4&status=hot", 6, 6},
		{"resource_id=resource-21", 1, 1},
		{"resource_id=resource-2", 0, 0},
		{"q=not-present", 0, 0},
		{"status=abnormal", 1, 1},
		{"status=open", 1, 1},
		{"status=degraded", 0, 0},
	} {
		data := root.call("GET", "/api/admin/resource-instances?"+test.query, nil, 200)
		if data["total"] != float64(test.total) || len(data["items"].([]any)) != test.count {
			t.Fatalf("incorrect search for %s: %+v", test.query, data)
		}
	}
	logs := []model.Log{
		{UserID: 100, ResourceID: "resource-21", Model: "Alpha", TraceID: "direct"},
		{UserID: 200, ResourceIDs: `["resource-21","other"]`, Model: "Alpha", TraceID: "attributed"},
		{UserID: 100, ResourceIDs: `["resource-210"]`, Model: "Alpha", TraceID: "prefix-must-not-match"},
		{UserID: 200, ResourceIDs: "legacy-invalid", Model: "Alpha", TraceID: "legacy"},
		{UserID: 100, ResourceID: "resource-21", ResourceIDs: `["resource-21"]`, Model: "Beta", TraceID: "deduplicated"},
	}
	if err := model.DB.Create(&logs).Error; err != nil {
		t.Fatal(err)
	}
	for page, trace := range []string{"attributed", "direct"} {
		data := root.call("GET", fmt.Sprintf("/api/log?resource_id=resource-21&model=Alpha&page_size=1&p=%d", page+1), nil, 200)
		items := data["items"].([]any)
		if data["total"] != float64(2) || len(items) != 1 || items[0].(map[string]any)["trace_id"] != trace {
			t.Fatalf("incorrect paged resource calls: %+v", data)
		}
	}
	data := root.call("GET", "/api/log?resource_id=resource-21", nil, 200)
	if data["total"] != float64(3) {
		t.Fatalf("resource calls duplicated or matched by prefix: %+v", data)
	}
	rows, total, err := model.ListResourceLogs(100, 1, 20, "Alpha", "resource-21")
	if err != nil || total != 1 || len(rows) != 1 || rows[0].TraceID != "direct" {
		t.Fatalf("owner boundary lost: %+v %d %v", rows, total, err)
	}
}

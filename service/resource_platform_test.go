package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"starhub/metric"
	"starhub/model"
)

func resourceTestDB(t *testing.T) {
	t.Helper()
	if err := model.InitDB(filepath.Join(t.TempDir(), "resource.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = model.CloseDB() })
}

func TestObservationResultRetryDoesNotReplayUpstream(t *testing.T) {
	resourceTestDB(t)
	metric.Reset()
	t.Setenv("NODE_NAME", "test-node")
	var upstreamCalls, results atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":1}}\n\ndata: [DONE]\n\n")
	}))
	defer upstream.Close()
	ch := &model.Channel{Name: "probe", Models: "test", Group: "default", Status: 1, Type: 1, Key: "never-log-me", BaseURL: upstream.URL}
	if err := model.InsertChannel(ch); err != nil {
		t.Fatal(err)
	}
	work := model.ResourceObservationWork{ProbeID: uuid.NewString(), ResourceKey: fmt.Sprintf("%d:test", ch.ID), ChannelID: ch.ID, ModelName: "test", EndpointGeneration: model.ChannelEndpointGeneration(ch), Phase: "claiming", CreatedAt: time.Now().Unix()}
	if err := model.DB.Create(&work).Error; err != nil {
		t.Fatal(err)
	}
	platform := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Protection-Node") != "test-node" {
			t.Error("missing node identity")
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/claim") {
			task := resourceObservationTask{ProtocolVersion: 2, LeaseStartedAt: time.Now().Unix(), LeaseUntil: time.Now().Add(time.Minute).Unix(), ResourceKey: work.ResourceKey, Epoch: uuid.NewString(), ProbeID: work.ProbeID, EndpointGeneration: work.EndpointGeneration, TTFTInputCapTokens: 2000, TTFTTimingSchema: "content_event_v2"}
			_ = json.NewEncoder(w).Encode(map[string]any{"task": task})
			return
		}
		var result resourceObservationResult
		if err := json.NewDecoder(r.Body).Decode(&result); err != nil {
			t.Error(err)
		}
		if result.Outcome != "success" || result.TTFTMS <= 0 || result.TTFTInputTokens != 5 || result.TTFTSource != "content_event_v2" {
			t.Errorf("bad observation: %+v", result)
		}
		if results.Add(1) == 1 {
			w.WriteHeader(503)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"accepted": true, "probe_id": work.ProbeID})
	}))
	defer platform.Close()
	t.Setenv("RESOURCE_CATALOG_NOTIFY_URL", platform.URL)
	cursor := 0
	if err := runResourceObservationOnce(t.Context(), &cursor); err == nil {
		t.Fatal("expected delivery failure")
	}
	if err := runResourceObservationOnce(t.Context(), &cursor); err != nil {
		t.Fatal(err)
	}
	if upstreamCalls.Load() != 1 || results.Load() != 2 {
		t.Fatalf("probe replayed: upstream=%d result=%d", upstreamCalls.Load(), results.Load())
	}
	var logs int64
	model.DB.Model(&model.Log{}).Count(&logs)
	if logs != 0 || len(metric.PlatformMetricRows(true, false, 0, time.Now().Unix())) != 0 {
		t.Fatal("probe entered customer logs or production metrics")
	}
}

func TestResourceOutboxRequiresMatchingAcknowledgement(t *testing.T) {
	resourceTestDB(t)
	id := uuid.NewString()
	row := model.ResourceEventOutbox{ID: id, Path: "/internal/routing/protection-events", Payload: `{"event_id":"` + id + `"}`, CreatedAt: time.Now().Unix()}
	if err := model.DB.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"accepted":true,"event_id":"different"}`)
	}))
	defer server.Close()
	t.Setenv("RESOURCE_CATALOG_NOTIFY_URL", server.URL)
	if err := DeliverResourceEventsOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	model.DB.First(&row, "id = ?", id)
	if row.DeliveredAt != 0 || row.Attempts != 1 || row.NextAttemptAt <= time.Now().Unix() || row.LastError == "" {
		t.Fatalf("bad retry state: %+v", row)
	}
}

func TestHardFailureBatchPreservesSequenceAcrossRetries(t *testing.T) {
	resourceTestDB(t)
	for i := 3; i >= 1; i-- {
		id := fmt.Sprintf("event-%d", i)
		if err := model.DB.Create(&model.ResourceEventOutbox{ID: id, StreamStartedAt: 1, StreamSequence: int64(i), Payload: fmt.Sprintf(`{"event_id":%q}`, id)}).Error; err != nil {
			t.Fatal(err)
		}
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/internal/routing/hard-failure-events" {
			t.Error(r.URL.Path)
		}
		var rows []struct {
			ID string `json:"event_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&rows); err != nil {
			t.Error(err)
		}
		if len(rows) != 3 || rows[0].ID != "event-1" || rows[2].ID != "event-3" {
			t.Errorf("unordered batch: %+v", rows)
		}
		if calls == 1 {
			w.WriteHeader(503)
			return
		}
		fmt.Fprint(w, `[{"event_id":"event-1","accepted":true},{"event_id":"event-2","accepted":true},{"event_id":"event-3","accepted":true}]`)
	}))
	defer server.Close()
	t.Setenv("RESOURCE_CATALOG_NOTIFY_URL", server.URL)
	if err := DeliverResourceEventsOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := DeliverResourceEventsOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("retry bypassed sequence backoff")
	}
	model.DB.Model(&model.ResourceEventOutbox{}).Where("stream_sequence > 0").Update("next_attempt_at", 0)
	if err := DeliverResourceEventsOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	var pending int64
	model.DB.Model(&model.ResourceEventOutbox{}).Where("delivered_at=0").Count(&pending)
	if pending != 0 || calls != 2 {
		t.Fatalf("pending=%d calls=%d", pending, calls)
	}
}

func TestObservationConflictDoesNotBlockFutureWork(t *testing.T) {
	resourceTestDB(t)
	work := model.ResourceObservationWork{ProbeID: uuid.NewString(), Phase: "result", Result: `{}`, CreatedAt: time.Now().Unix()}
	if err := model.DB.Create(&work).Error; err != nil {
		t.Fatal(err)
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(409) }))
	defer server.Close()
	t.Setenv("RESOURCE_CATALOG_NOTIFY_URL", server.URL)
	cursor := 0
	if err := runResourceObservationOnce(t.Context(), &cursor); err != nil {
		t.Fatal(err)
	}
	if err := runResourceObservationOnce(t.Context(), &cursor); err != nil {
		t.Fatal(err)
	}
	model.DB.First(&work, "probe_id = ?", work.ProbeID)
	if calls != 1 || work.Phase != "rejected" || work.LastError == "" {
		t.Fatalf("stale lease blocked worker: %+v calls=%d", work, calls)
	}
}

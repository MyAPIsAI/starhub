package model

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestLatencyTransitionsQueueVersionedEvidenceAtomically(t *testing.T) {
	channels, _ := scoredFixture(t, 1)
	t.Setenv("RESOURCE_CATALOG_NOTIFY_URL", "http://resource.invalid")
	t.Setenv("NODE_NAME", "latency-node")
	ch, err := GetSatisfiedChannel("default", "test-model")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now()
	if err := DB.Exec(`CREATE TRIGGER fail_latency_outbox BEFORE INSERT ON resource_event_outboxes BEGIN SELECT RAISE(ABORT,'injected'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if err := RecordRoutingOutcome(ch.RuntimeRoute, true, 200, 20*time.Second, at); err == nil {
		t.Fatal("event failure did not roll back latency")
	}
	state, err := ReadLocalRoutingState(ch.RuntimeRoute.Item)
	if err != nil || state.Samples != 0 || state.PublishedLatencyVersion != 0 {
		t.Fatal("failed event left applied state")
	}
	DB.Exec(`DROP TRIGGER fail_latency_outbox`)
	for i := 0; i < 3; i++ {
		if err := RecordRoutingOutcome(ch.RuntimeRoute, true, 200, 20*time.Second, at); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 3; i++ {
		if err := RecordRoutingOutcome(ch.RuntimeRoute, true, 200, 100*time.Millisecond, at); err != nil {
			t.Fatal(err)
		}
	}
	state, err = ReadLocalRoutingState(ch.RuntimeRoute.Item)
	if err != nil || state.Degraded || state.LatencyVersion != 3 || state.PublishedLatencyVersion != 3 {
		t.Fatalf("wrong transition state: %+v %v", state, err)
	}
	var rows []ResourceEventOutbox
	if err := DB.Order("created_at,id").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("transition events=%d", len(rows))
	}
	events := make([]json.RawMessage, 3)
	for _, row := range rows {
		var event struct {
			Cause   string `json:"cause"`
			Latency struct {
				Version  int  `json:"version"`
				Degraded bool `json:"degraded"`
			} `json:"latency"`
		}
		if json.Unmarshal([]byte(row.Payload), &event) != nil || event.Latency.Version < 1 || event.Latency.Version > 3 {
			t.Fatal("bad latency payload")
		}
		events[event.Latency.Version-1] = json.RawMessage(row.Payload)
	}
	if fixture := os.Getenv("STARHUB_LATENCY_FIXTURE"); fixture != "" {
		raw, _ := json.Marshal(events)
		if err := os.WriteFile(fixture, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	_ = channels
}

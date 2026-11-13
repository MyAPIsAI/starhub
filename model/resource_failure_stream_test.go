package model

import (
	"encoding/json"
	"testing"
	"time"
)

func TestFailureStreamFencesInflightAttemptsAndRestart(t *testing.T) {
	channels, _ := scoredFixture(t, 1)
	t.Setenv("RESOURCE_CATALOG_NOTIFY_URL", "http://resource.invalid")
	t.Setenv("NODE_NAME", "gateway-test")
	ch, err := GetSatisfiedChannel("default", channels[0].Models)
	if err != nil {
		t.Fatal(err)
	}
	a := ch.RuntimeRoute
	BeginRoutingAttempt(a)
	a.Sent = true
	if err := QueueRoutingWatermark(time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := EndRoutingAttempt(a, "request-test", "upstream_unavailable", 503, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := QueueRoutingWatermark(time.Now()); err != nil {
		t.Fatal(err)
	}
	var rows []ResourceEventOutbox
	if err := DB.Order("stream_started_at, stream_sequence").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("events: %d", len(rows))
	}
	for i, row := range rows {
		if row.StreamSequence != int64(i+1) {
			t.Fatalf("sequence: %+v", row)
		}
	}
	var first, terminal struct {
		HardFailure struct {
			Watermark int64 `json:"watermark"`
			Terminal  struct {
				AttemptID string `json:"attempt_id"`
				Sent      bool   `json:"sent"`
			} `json:"terminal"`
		} `json:"hard_failure"`
	}
	_ = json.Unmarshal([]byte(rows[0].Payload), &first)
	_ = json.Unmarshal([]byte(rows[1].Payload), &terminal)
	if first.HardFailure.Watermark >= a.StartedAt.UnixMilli() || terminal.HardFailure.Terminal.AttemptID != a.AttemptID || !terminal.HardFailure.Terminal.Sent {
		t.Fatal("invalid inflight fence or terminal identity")
	}
	ResetRoutingFailureStream()
	if err := QueueRoutingWatermark(time.Now()); err != nil {
		t.Fatal(err)
	}
	var restarted ResourceEventOutbox
	DB.Order("stream_started_at DESC, stream_sequence DESC").First(&restarted)
	if restarted.StreamStartedAt <= rows[2].StreamStartedAt || restarted.StreamSequence != 1 {
		t.Fatal("restart reused an incomplete stream")
	}
}

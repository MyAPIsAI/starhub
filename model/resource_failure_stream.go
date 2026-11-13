package model

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ResourceFailureStream struct {
	ID        int `gorm:"primaryKey"`
	StreamID  string
	Sequence  int64
	StartedAt int64
}

var failureStreamState struct {
	sync.Mutex
	rotate   bool
	inflight map[string]time.Time
}

func BeginRoutingAttempt(attempt *RoutingAttempt) {
	if attempt == nil {
		return
	}
	failureStreamState.Lock()
	defer failureStreamState.Unlock()
	if failureStreamState.inflight == nil {
		failureStreamState.inflight = map[string]time.Time{}
	}
	attempt.AttemptID = uuid.NewString()
	attempt.StartedAt = time.Now()
	failureStreamState.inflight[attempt.AttemptID] = attempt.StartedAt
}

func EndRoutingAttempt(attempt *RoutingAttempt, requestID, class string, status int, now time.Time) error {
	if attempt == nil || attempt.AttemptID == "" {
		return nil
	}
	failureStreamState.Lock()
	defer failureStreamState.Unlock()
	defer delete(failureStreamState.inflight, attempt.AttemptID)
	if os.Getenv("RESOURCE_CATALOG_NOTIFY_URL") == "" {
		return nil
	}
	if requestID == "" {
		requestID = attempt.AttemptID
	}
	item := attempt.Item
	generation := ""
	epoch := int64(0)
	if item.Protection != nil {
		generation = item.Protection.EndpointGeneration
		epoch = item.Protection.RecoveryEpoch
	}
	terminal := map[string]any{"attempt_id": attempt.AttemptID, "request_id": requestID, "resource_key": RoutingResourceKey(item), "endpoint_generation": generation, "recovery_epoch": epoch, "policy_fingerprint": item.PolicyFingerprint, "scope": "channel_model", "sent": attempt.Sent, "raw_status": status, "class": class, "started_at": attempt.StartedAt.UnixMilli(), "ended_at": now.UnixMilli()}
	return enqueueFailurePacket(terminal, attempt.AttemptID, 0, now)
}

// A watermark never advances past an unfinished attempt. The stream sequence
// and outbox write commit together, so transport retries cannot create gaps.
func QueueRoutingWatermark(now time.Time) error {
	if os.Getenv("RESOURCE_CATALOG_NOTIFY_URL") == "" {
		return nil
	}
	failureStreamState.Lock()
	defer failureStreamState.Unlock()
	watermark := now.UnixMilli()
	for _, start := range failureStreamState.inflight {
		watermark = min(watermark, start.UnixMilli()-1)
	}
	return enqueueFailurePacket(nil, uuid.NewString(), watermark, now)
}

func enqueueFailurePacket(terminal map[string]any, attemptID string, watermark int64, now time.Time) error {
	source := os.Getenv("ROUTING_PROTECTION_SOURCE")
	if source == "" {
		source = "talen"
	}
	node := os.Getenv("NODE_NAME")
	err := DB.Transaction(func(tx *gorm.DB) error {
		stream := ResourceFailureStream{ID: 1, StreamID: uuid.NewString(), StartedAt: now.UnixMilli()}
		if err := tx.Where("id = 1").FirstOrCreate(&stream).Error; err != nil {
			return err
		}
		if failureStreamState.rotate {
			stream.StreamID = uuid.NewString()
			stream.Sequence = 0
			stream.StartedAt = max(now.UnixMilli(), stream.StartedAt+1)
		}
		stream.Sequence++
		id := fmt.Sprintf("hf:%x", sha256.Sum256([]byte(source+"\n"+attemptID)))[:63]
		packet := map[string]any{"schema": 1, "source": source, "node": node, "stream_id": stream.StreamID, "sequence": stream.Sequence, "stream_started_at": stream.StartedAt, "watermark": watermark}
		if terminal != nil {
			packet["terminal"] = terminal
		}
		event := map[string]any{"schema": 1, "event_id": id, "source": source, "instance_id": node, "instance_boot_id": routingBootID, "cause": "hard_failure_stream", "observed_at": now.Unix(), "hard_failure": packet}
		raw, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if err := tx.Create(&ResourceEventOutbox{ID: id, Path: "/internal/routing/hard-failure-events", Payload: string(raw), CreatedAt: now.Unix(), StreamStartedAt: stream.StartedAt, StreamSequence: stream.Sequence}).Error; err != nil {
			return err
		}
		return tx.Save(&stream).Error
	})
	failureStreamState.rotate = err != nil
	return err
}

func ResetRoutingFailureStream() {
	failureStreamState.Lock()
	defer failureStreamState.Unlock()
	// A restart cannot attest to terminals from requests interrupted by the crash.
	failureStreamState.rotate = true
	failureStreamState.inflight = map[string]time.Time{}
}

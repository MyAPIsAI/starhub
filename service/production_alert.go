package service

import (
	"sync"
	"time"
)

var productionAlerts = struct {
	sync.Mutex
	nextCheck                  time.Time
	last                       map[string]time.Time
	rejected, expired, dropped int64
}{last: map[string]time.Time{}}

func productionAlertReasons(h ProductionTelemetryStatus, now time.Time) []string {
	reasons := []string{}
	if h.Collector.StartedAt == 0 {
		reasons = append(reasons, "collector_not_started")
	}
	if !h.Durable.Available {
		if h.Durable.ErrorKind == "timeout" {
			reasons = append(reasons, "storage_query_timeout")
		} else {
			reasons = append(reasons, "storage_unavailable")
		}
	}
	if h.PersistenceBlocked {
		reasons = append(reasons, "persistence_blocked")
	}
	if h.Stage.Failing || h.Flush.Failing {
		reasons = append(reasons, "persistence_failed")
	}
	if h.Cleanup.Failing {
		reasons = append(reasons, "retention_failed")
	}
	if h.Collector.EstimatedBytes >= h.Collector.MaxBytes*9/10 && h.Collector.MaxBytes > 0 || h.Collector.Retained >= 90000 {
		reasons = append(reasons, "collector_capacity")
	}
	s := h.Durable.Storage
	if h.Durable.Available && (s.MaxRows > 0 && s.Rows >= s.MaxRows*9/10 || s.MaxBytes > 0 && s.Bytes >= s.MaxBytes*9/10) {
		reasons = append(reasons, "storage_capacity")
	}
	if h.Collector.StartedAt > 0 && now.Unix()-h.Collector.StartedAt > 180 && (h.Durable.LastPublishedMinute == 0 || now.Unix()-h.Durable.LastPublishedMinute > 180) {
		reasons = append(reasons, "publication_stalled")
	}
	if h.DeliveryEnabled {
		if h.Delivery.Failing {
			reasons = append(reasons, "delivery_failed")
		}
		if h.Durable.Available && h.OldestPendingAgeSeconds > 300 {
			reasons = append(reasons, "delivery_backlog")
		}
	}
	if (h.SnapshotRequired || h.DeliveryEnabled) && h.SnapshotStale {
		reasons = append(reasons, "routing_snapshot_stale")
	}
	return reasons
}

// Only the worker emits alerts. Polling monitoring endpoints never performs
// webhook I/O, and each static reason is throttled independently.
func checkProductionHealthAlerts(now time.Time) {
	productionAlerts.Lock()
	defer productionAlerts.Unlock()
	if now.Before(productionAlerts.nextCheck) {
		return
	}
	productionAlerts.nextCheck = now.Add(5 * time.Second)
	h := ProductionTelemetryHealth(now)
	reasons := productionAlertReasons(h, now)
	if h.Collector.Rejected > productionAlerts.rejected {
		reasons = append(reasons, "collector_rejected_attempts")
	}
	if h.Collector.Expired > productionAlerts.expired {
		reasons = append(reasons, "collector_expired_attempts")
	}
	if h.Durable.Storage.UndeliveredDropped > productionAlerts.dropped {
		reasons = append(reasons, "undelivered_attempts_expired")
	}
	productionAlerts.rejected = h.Collector.Rejected
	productionAlerts.expired = h.Collector.Expired
	productionAlerts.dropped = h.Durable.Storage.UndeliveredDropped
	for _, reason := range reasons {
		if now.Sub(productionAlerts.last[reason]) < time.Minute {
			continue
		}
		productionAlerts.last[reason] = now
		deliverAlert(AlertEvent{Type: "telemetry_health", Message: reason, Severity: "warning", Timestamp: now.Unix()})
	}
}

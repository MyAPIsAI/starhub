package service

import (
	"context"
	"os"
	"starhub/metric"
	"starhub/model"
	"starhub/setting"
	"sync"
	"time"

	"gorm.io/gorm"
)

type ProductionOperationStatus struct {
	Calls          int64   `json:"calls"`
	Errors         int64   `json:"errors"`
	LastSuccess    int64   `json:"last_success_at"`
	LastFailure    int64   `json:"last_failure_at"`
	LastDurationMS float64 `json:"last_duration_ms"`
	MaxDurationMS  float64 `json:"max_duration_ms"`
	Failing        bool    `json:"failing"`
}

type productionOperation struct {
	sync.Mutex
	status    ProductionOperationStatus
	durations metric.DurationHistogram
}

func (o *productionOperation) finish(start time.Time, err error) {
	o.Lock()
	defer o.Unlock()
	s := &o.status
	o.durations.Observe(time.Since(start))
	s.Calls++
	s.LastDurationMS = float64(time.Since(start).Microseconds()) / 1000
	s.MaxDurationMS = max(s.MaxDurationMS, s.LastDurationMS)
	s.Failing = err != nil
	if err != nil {
		s.Errors++
		s.LastFailure = time.Now().Unix()
	} else {
		s.LastSuccess = time.Now().Unix()
	}
}

func (o *productionOperation) snapshot() ProductionOperationStatus {
	o.Lock()
	defer o.Unlock()
	return o.status
}

var productionStageHealth, productionFlushHealth, productionDeliveryHealth, productionCleanupHealth productionOperation

type ProductionDurableHealth struct {
	ObservedAt          int64                        `json:"observed_at"`
	Available           bool                         `json:"available"`
	Storage             model.ProductionStorageState `json:"storage"`
	Pending             int64                        `json:"undelivered_attempts"`
	Staged              int64                        `json:"unpublished_attempts"`
	OldestPending       int64                        `json:"oldest_pending_minute"`
	LastPublishedMinute int64                        `json:"last_published_minute"`
	LastPublishedAt     int64                        `json:"last_published_at"`
}

var productionHealthCache = struct {
	sync.Mutex
	db      *gorm.DB
	expires time.Time
	value   ProductionDurableHealth
}{}

func durableProductionHealth(now time.Time) ProductionDurableHealth {
	productionHealthCache.Lock()
	defer productionHealthCache.Unlock()
	if productionHealthCache.db == model.DB && now.Before(productionHealthCache.expires) {
		return productionHealthCache.value
	}
	h := ProductionDurableHealth{ObservedAt: now.Unix()}
	if model.DB != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
		defer cancel()
		db := model.DB.WithContext(ctx)
		err := db.First(&h.Storage, 1).Error
		if err == nil {
			var pending struct{ Pending, OldestPending int64 }
			err = db.Raw(`SELECT COUNT(*) AS pending, COALESCE(MIN(minute),0) AS oldest_pending FROM production_attempt_records WHERE delivered_at=0`).Scan(&pending).Error
			h.Pending, h.OldestPending = pending.Pending, pending.OldestPending
		}
		if err == nil {
			err = db.Model(&model.ProductionAttemptRecord{}).Where("batch_id != '' AND NOT EXISTS (SELECT 1 FROM production_telemetry_minutes AS published WHERE published.minute=production_attempt_records.minute AND published.batch_id=production_attempt_records.batch_id)").Count(&h.Staged).Error
		}
		if err == nil {
			var latest []model.ProductionTelemetryMinute
			err = db.Select("minute,created_at").Order("minute DESC").Limit(1).Find(&latest).Error
			if len(latest) > 0 {
				h.LastPublishedMinute = latest[0].Minute
				h.LastPublishedAt = latest[0].CreatedAt
			}
		}
		h.Available = err == nil
	}
	productionHealthCache.db = model.DB
	productionHealthCache.expires = now.Add(5 * time.Second)
	productionHealthCache.value = h
	return h
}

type ProductionTelemetryStatus struct {
	ConfirmedAttempts       int64                              `json:"confirmed_attempts_since_start"`
	RetriedAttempts         int64                              `json:"retried_attempts_since_start"`
	SnapshotRequired        bool                               `json:"snapshot_required"`
	Timings                 map[string]metric.DurationSnapshot `json:"timings_us"`
	Collector               metric.ProductionCollectorHealth   `json:"collector"`
	Durable                 ProductionDurableHealth            `json:"durable"`
	Stage                   ProductionOperationStatus          `json:"stage"`
	Flush                   ProductionOperationStatus          `json:"flush"`
	Delivery                ProductionOperationStatus          `json:"delivery"`
	Cleanup                 ProductionOperationStatus          `json:"cleanup"`
	DeliveryEnabled         bool                               `json:"delivery_enabled"`
	OldestPendingAgeSeconds int64                              `json:"oldest_pending_age_seconds"`
	SnapshotReady           bool                               `json:"snapshot_ready"`
	SnapshotAgeSeconds      int64                              `json:"snapshot_age_seconds"`
	SnapshotStale           bool                               `json:"snapshot_stale"`
	CacheEntries            int                                `json:"cache_entries"`
	CacheBytes              int                                `json:"cache_bytes"`
	CacheHits               int64                              `json:"cache_hits"`
	CacheMisses             int64                              `json:"cache_misses"`
	ActiveRequests          int64                              `json:"active_requests"`
	ActiveAttempts          int64                              `json:"active_attempts"`
}

// Monitoring is read-only. Durable counts are shared for five seconds to bound
// repeated polling; observed_at lets clients identify the age of those counts.
func ProductionTelemetryHealth(now time.Time) ProductionTelemetryStatus {
	h := ProductionTelemetryStatus{Collector: metric.ProductionCollectorSnapshot(), Durable: durableProductionHealth(now), Stage: productionStageHealth.snapshot(), Flush: productionFlushHealth.snapshot(), Delivery: productionDeliveryHealth.snapshot(), Cleanup: productionCleanupHealth.snapshot(), DeliveryEnabled: os.Getenv("RESOURCE_CATALOG_NOTIFY_URL") != "", CacheHits: productionCacheHits.Load(), CacheMisses: productionCacheMisses.Load()}
	_, age, ready := model.RoutingSnapshotStatus()
	h.SnapshotReady = ready
	h.SnapshotRequired = setting.IsCommercialMode()
	h.ConfirmedAttempts = productionDeliveryConfirmed.Load()
	h.RetriedAttempts = productionDeliveryRetried.Load()
	h.SnapshotAgeSeconds = max(0, int64(age.Seconds()))
	h.SnapshotStale = !ready || age > 3*time.Minute
	if h.Durable.OldestPending > 0 {
		h.OldestPendingAgeSeconds = max(0, now.Unix()-h.Durable.OldestPending)
	}
	h.ActiveRequests, h.ActiveAttempts = metric.InflightCounts()
	h.Timings = metric.ProductionTimings()
	for name, value := range model.LocalRoutingTimings() {
		h.Timings[name] = value
	}
	h.Timings["stage"] = productionStageHealth.durations.Snapshot()
	h.Timings["flush"] = productionFlushHealth.durations.Snapshot()
	h.Timings["delivery"] = productionDeliveryHealth.durations.Snapshot()
	h.Timings["cleanup"] = productionCleanupHealth.durations.Snapshot()
	productionCache.Lock()
	h.CacheEntries = productionCache.order.Len()
	h.CacheBytes = productionCache.bytes
	productionCache.Unlock()
	return h
}

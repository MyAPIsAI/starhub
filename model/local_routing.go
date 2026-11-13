package model

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type RoutingAttempt struct {
	AttemptID         string
	Sent              bool
	Item              RouteItem
	FailureGeneration uint64
	StartedAt         time.Time
}

type LocalRoutingState struct {
	LatencyThreshold        float64 `json:"latency_threshold_ms"`
	LatencyVersion          int64   `json:"latency_version"`
	PublishedLatencyVersion int64   `json:"published_latency_version"`
	ResourceKey             string  `gorm:"primaryKey" json:"resource_key"`
	EndpointGeneration      string  `json:"endpoint_generation"`
	RecoveryEpoch           int64   `json:"recovery_epoch"`
	FailureGeneration       uint64  `json:"failure_generation"`
	ConsecutiveFailures     int     `json:"consecutive_failures"`
	HalfOpenFailures        int     `json:"half_open_failures"`
	OpenUntil               int64   `json:"open_until"`
	ProbeLeaseUntil         int64   `json:"probe_lease_until"`
	ProbeID                 string  `json:"probe_id"`
	BackoffSeconds          int     `json:"backoff_seconds"`
	EWMA                    float64 `json:"ewma_ms"`
	Samples                 int     `json:"samples"`
	Degraded                bool    `json:"degraded"`
	HealthySince            int64   `json:"healthy_since"`
	HealthyStreak           int     `json:"healthy_streak"`
	LastCause               string  `json:"last_cause"`
}

type ResourceEventOutbox struct {
	ID              string `gorm:"primaryKey" json:"event_id"`
	Path            string `json:"path"`
	Payload         string `gorm:"type:text" json:"-"`
	CreatedAt       int64  `json:"created_at"`
	Attempts        int    `json:"attempts"`
	NextAttemptAt   int64  `json:"next_attempt_at"`
	DeliveredAt     int64  `json:"delivered_at"`
	LastError       string `json:"last_error"`
	StreamStartedAt int64  `gorm:"index" json:"stream_started_at,omitempty"`
	StreamSequence  int64  `json:"stream_sequence,omitempty"`
}

var localRoutingMu sync.Mutex
var routingBootID = uuid.NewString()

type routingExecution struct {
	CircuitFailureThreshold  int     `json:"circuit_failure_threshold"`
	CircuitFailureThresholdU int     `json:"circuit_failure_threshold_u"`
	CircuitOpenBaseSeconds   int     `json:"circuit_open_base_seconds"`
	CircuitOpenMaxSeconds    int     `json:"circuit_open_max_seconds"`
	EWMAAlpha                float64 `json:"ewma_alpha"`
	EWMAMinSamples           int     `json:"ewma_min_samples"`
	EWMAOverrunS             float64 `json:"ewma_overrun_s"`
	EWMAOverrunU             float64 `json:"ewma_overrun_u"`
}

func localExecution(item RouteItem) (routingExecution, float64) {
	e := routingExecution{5, 2, 30, 300, .5, 3, 1.3, 1}
	threshold := map[string]float64{"S0": 7000, "S1": 9000, "S2": 12000}[item.QualityLevel]
	if threshold == 0 {
		threshold = 12000
	}
	routingSnapshotState.RLock()
	defer routingSnapshotState.RUnlock()
	if s := routingSnapshotState.snapshot; s != nil {
		var policy struct {
			Execution json.RawMessage `json:"execution"`
			Levels    map[string]struct {
				TTFT float64 `json:"ttft_p95_max_ms"`
			} `json:"level_thresholds"`
		}
		if json.Unmarshal(s.Policy, &policy) == nil {
			if len(policy.Execution) > 0 {
				_ = json.Unmarshal(policy.Execution, &e)
			}
			key := map[string]string{"S0": "s0", "S1": "s1", "S2": "s2"}[item.QualityLevel]
			if level := policy.Levels[key]; level.TTFT > 0 {
				threshold = level.TTFT
			}
		}
	}
	// Bounds preserve a finite circuit and safe arithmetic even on older policies.
	e.CircuitFailureThreshold = max(1, min(e.CircuitFailureThreshold, 100))
	e.CircuitFailureThresholdU = max(1, min(e.CircuitFailureThresholdU, 100))
	e.CircuitOpenBaseSeconds = max(30, min(e.CircuitOpenBaseSeconds, 300))
	e.CircuitOpenMaxSeconds = max(e.CircuitOpenBaseSeconds, min(e.CircuitOpenMaxSeconds, 300))
	if e.EWMAAlpha <= 0 || e.EWMAAlpha > 1 {
		e.EWMAAlpha = .5
	}
	e.EWMAMinSamples = max(1, e.EWMAMinSamples)
	if e.EWMAOverrunS < 1 {
		e.EWMAOverrunS = 1.3
	}
	if e.EWMAOverrunU < 1 {
		e.EWMAOverrunU = 1
	}
	return e, threshold
}

func readLocalRoutingState(db *gorm.DB, item RouteItem) (LocalRoutingState, error) {
	state := LocalRoutingState{ResourceKey: RoutingResourceKey(item)}
	err := db.First(&state, "resource_key = ?", state.ResourceKey).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return state, err
	}
	if p := item.Protection; p != nil {
		if state.EndpointGeneration != p.EndpointGeneration || state.RecoveryEpoch != p.RecoveryEpoch {
			state = LocalRoutingState{ResourceKey: RoutingResourceKey(item), EndpointGeneration: p.EndpointGeneration, RecoveryEpoch: p.RecoveryEpoch}
		}
	}
	return state, nil
}

func ReadLocalRoutingState(item RouteItem) (LocalRoutingState, error) {
	return readLocalRoutingState(DB, item)
}

// RecordRoutingOutcome fences successes by the failure generation captured at
// selection. A late success cannot close a circuit opened by a newer failure.
func RecordRoutingOutcome(attempt *RoutingAttempt, success bool, status int, ttft time.Duration, now time.Time) error {
	if attempt == nil {
		return nil
	}
	if !success && status != 0 && status != 401 && status != 403 && status != 404 && status != 408 && status != 429 && status < 500 {
		return nil
	}
	policy, threshold := localExecution(attempt.Item)
	routingSnapshotState.RLock()
	defer routingSnapshotState.RUnlock()
	if !routingItemCurrentLocked(attempt.Item) {
		return nil
	}
	localRoutingMu.Lock()
	defer localRoutingMu.Unlock()
	return DB.Transaction(func(tx *gorm.DB) error {
		state, err := readLocalRoutingState(tx, attempt.Item)
		if err != nil {
			return err
		}
		if success {
			if state.FailureGeneration != attempt.FailureGeneration || state.OpenUntil > 0 {
				return nil
			}
			state.ConsecutiveFailures = 0
			updateRoutingLatency(&state, attempt.Item, policy, threshold, ttft, now)
		} else {
			state.FailureGeneration++
			state.ConsecutiveFailures++
			state.HealthyStreak = 0
			state.HealthySince = 0
			state.LastCause = "upstream_failure"
			limit := policy.CircuitFailureThreshold
			if qualityRank(attempt.Item.QualityLevel) == 0 {
				limit = policy.CircuitFailureThresholdU
			}
			if status == 401 || status == 403 {
				state.LastCause = "credential_error"
				limit = 1
			}
			if status == 404 {
				state.LastCause = "model_not_found"
				limit = 1
			}
			if state.ConsecutiveFailures >= limit {
				state.BackoffSeconds = max(state.BackoffSeconds, policy.CircuitOpenBaseSeconds)
				state.OpenUntil = now.Add(time.Duration(state.BackoffSeconds) * time.Second).Unix()
			}
		}
		if err := publishLatencyTransition(tx, attempt.Item, &state, threshold, now); err != nil {
			return err
		}
		return tx.Save(&state).Error
	})
}

func updateRoutingLatency(state *LocalRoutingState, item RouteItem, policy routingExecution, threshold float64, ttft time.Duration, now time.Time) {
	before := state.Degraded
	if ttft > 0 {
		ms := float64(ttft.Microseconds()) / 1000
		if state.Samples == 0 {
			state.EWMA = ms
		} else {
			state.EWMA += policy.EWMAAlpha * (ms - state.EWMA)
		}
		state.Samples = min(state.Samples+1, 1000000000)
		overrun := policy.EWMAOverrunS
		if qualityRank(item.QualityLevel) == 0 {
			overrun = policy.EWMAOverrunU
		}
		if state.Samples >= policy.EWMAMinSamples && state.EWMA > threshold*overrun {
			state.Degraded = true
			state.HealthySince = 0
			state.HealthyStreak = 0
			state.LastCause = "ewma_latency"
		} else if state.Degraded && ms <= threshold {
			if state.HealthySince == 0 {
				state.HealthySince = now.Unix()
			}
			state.HealthyStreak++
			if state.HealthyStreak >= 3 || (now.Unix()-state.HealthySince >= 60 && state.EWMA <= threshold) {
				state.Degraded = false
				state.HealthyStreak = 0
				state.HealthySince = 0
				state.LastCause = "ewma_recovered"
			}
		} else if state.Degraded {
			state.HealthySince = 0
			state.HealthyStreak = 0
		}
	}
	if before != state.Degraded {
		state.LatencyVersion++
	}
}

type RoutingProbe struct {
	Item  RouteItem
	State LocalRoutingState
}

func ClaimLocalRoutingProbes(now time.Time) ([]RoutingProbe, error) {
	routingSnapshotState.RLock()
	items := append([]RouteItem(nil), routingSnapshotState.items...)
	routingSnapshotState.RUnlock()
	localRoutingMu.Lock()
	defer localRoutingMu.Unlock()
	result := []RoutingProbe{}
	err := DB.Transaction(func(tx *gorm.DB) error {
		seen := map[string]bool{}
		for _, item := range items {
			key := RoutingResourceKey(item)
			if seen[key] {
				continue
			}
			seen[key] = true
			state, err := readLocalRoutingState(tx, item)
			if err != nil {
				return err
			}
			if (state.OpenUntil == 0 && !state.Degraded) || state.OpenUntil > now.Unix() || state.ProbeLeaseUntil > now.Unix() {
				continue
			}
			if item.Protection != nil && (item.Protection.CatalogExcluded || item.Protection.Mode == "manual" || item.Protection.Mode == "disabled") {
				continue
			}
			state.ProbeLeaseUntil = now.Add(35 * time.Second).Unix()
			state.ProbeID = uuid.NewString()
			if err := tx.Save(&state).Error; err != nil {
				return err
			}
			result = append(result, RoutingProbe{item, state})
			if len(result) == 4 {
				break
			}
		}
		return nil
	})
	return result, err
}

func CompleteLocalRoutingProbe(probe RoutingProbe, success bool, now time.Time, latency ...time.Duration) error {
	policy, threshold := localExecution(probe.Item)
	routingSnapshotState.RLock()
	defer routingSnapshotState.RUnlock()
	if !routingItemCurrentLocked(probe.Item) {
		return nil
	}
	localRoutingMu.Lock()
	defer localRoutingMu.Unlock()
	return DB.Transaction(func(tx *gorm.DB) error {
		state, err := readLocalRoutingState(tx, probe.Item)
		if err != nil {
			return err
		}
		if state.ProbeID != probe.State.ProbeID || state.FailureGeneration != probe.State.FailureGeneration || state.ProbeLeaseUntil < now.Unix() {
			return nil
		}
		if state.OpenUntil == 0 && state.Degraded {
			state.ProbeLeaseUntil = now.Add(30 * time.Second).Unix()
			if success && len(latency) > 0 {
				updateRoutingLatency(&state, probe.Item, policy, threshold, latency[0], now)
			} else {
				state.HealthyStreak, state.HealthySince = 0, 0
			}
			if err := publishLatencyTransition(tx, probe.Item, &state, threshold, now); err != nil {
				return err
			}
			return tx.Save(&state).Error
		}
		state.ProbeLeaseUntil = 0
		cause := "half_open_failures"
		if success {
			state.OpenUntil = 0
			state.BackoffSeconds = 0
			state.ConsecutiveFailures = 0
			state.HalfOpenFailures = 0
			cause = "recovery_success"
		} else {
			state.FailureGeneration++
			state.HalfOpenFailures++
			state.BackoffSeconds = min(policy.CircuitOpenMaxSeconds, max(policy.CircuitOpenBaseSeconds, state.BackoffSeconds*2))
			state.OpenUntil = now.Add(time.Duration(state.BackoffSeconds) * time.Second).Unix()
		}
		state.LastCause = cause
		if err := tx.Save(&state).Error; err != nil {
			return err
		}
		return enqueueProtectionEvent(tx, probe.Item, state, cause, probe.State.ProbeID, now)
	})
}

func routingItemCurrentLocked(item RouteItem) bool {
	for _, current := range routingSnapshotState.items {
		if current.ResourceID != item.ResourceID {
			continue
		}
		if current.Protection == nil || item.Protection == nil {
			return current.Protection == nil && item.Protection == nil
		}
		return current.Protection.EndpointGeneration == item.Protection.EndpointGeneration && current.Protection.RecoveryEpoch == item.Protection.RecoveryEpoch
	}
	return false
}

func enqueueProtectionEvent(tx *gorm.DB, item RouteItem, state LocalRoutingState, cause, probeID string, now time.Time) error {
	if item.Protection == nil || os.Getenv("RESOURCE_CATALOG_NOTIFY_URL") == "" {
		return nil
	}
	source := os.Getenv("ROUTING_PROTECTION_SOURCE")
	if source == "" {
		source = "talen"
	}
	id := uuid.NewString()
	payload := map[string]any{"schema": 1, "event_id": id, "source": source, "instance_id": os.Getenv("NODE_NAME"), "instance_boot_id": routingBootID, "resource_key": state.ResourceKey, "endpoint_generation": state.EndpointGeneration, "recovery_epoch": state.RecoveryEpoch, "failure_generation": state.FailureGeneration, "probe_id": probeID, "policy_fingerprint": item.PolicyFingerprint, "cause": cause, "half_open_failures": min(state.HalfOpenFailures, 1000), "observed_at": now.Unix()}
	if cause == "ewma_latency" || cause == "ewma_recovered" {
		payload["half_open_failures"] = 0
		payload["latency"] = map[string]any{"schema": 1, "version": state.LatencyVersion, "ewma_ms": state.EWMA, "threshold_ms": state.LatencyThreshold, "samples": state.Samples, "degraded": state.Degraded}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return tx.Create(&ResourceEventOutbox{ID: id, Path: "/internal/routing/protection-events", Payload: string(raw), CreatedAt: now.Unix()}).Error
}

func publishLatencyTransition(tx *gorm.DB, item RouteItem, state *LocalRoutingState, threshold float64, now time.Time) error {
	if state.Samples > 0 && state.LatencyVersion == 0 {
		state.LatencyVersion = 1
	}
	if state.LatencyVersion <= state.PublishedLatencyVersion || item.Protection == nil || os.Getenv("RESOURCE_CATALOG_NOTIFY_URL") == "" {
		return nil
	}
	cause := "ewma_recovered"
	state.LatencyThreshold = threshold
	if state.Degraded {
		cause = "ewma_latency"
	}
	if err := enqueueProtectionEvent(tx, item, *state, cause, uuid.NewString(), now); err != nil {
		return err
	}
	state.PublishedLatencyVersion = state.LatencyVersion
	return nil
}

func ResourceEventRetryDelay(attempts int) time.Duration {
	return time.Duration(math.Min(300, math.Pow(2, float64(min(attempts, 9))))) * time.Second
}

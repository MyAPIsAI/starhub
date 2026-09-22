package service

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"starhub/metric"
	"starhub/setting"
)

// alertHTTPClient is isolated so tests can replace it.
var alertHTTPClient = &http.Client{Timeout: 10 * time.Second}

// AlertEvent is the payload delivered to every configured notifier.
type AlertEvent struct {
	Type      string `json:"type"`       // channel_disabled, channel_enabled, error_rate_threshold
	ChannelID int64  `json:"channel_id"`
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
}

// NotifyChannelDisabled reports a channel auto-disable. Called from
// DisableChannel so every disable path (probe, broken-key, manual) is
// covered.
func NotifyChannelDisabled(channelID int64, reason string) {
	deliverAlert(AlertEvent{
		Type:      "channel_disabled",
		ChannelID: channelID,
		Message:   reason,
		Timestamp: time.Now().Unix(),
	})
}

// NotifyChannelEnabled reports a channel auto-enable.
func NotifyChannelEnabled(channelID int64, reason string) {
	deliverAlert(AlertEvent{
		Type:      "channel_enabled",
		ChannelID: channelID,
		Message:   reason,
		Timestamp: time.Now().Unix(),
	})
}

// NotifyErrorRateThreshold fires when a channel's rolling error rate
// crosses the configured threshold. It is throttled per channel so a
// flapping channel does not flood the notifier.
func NotifyErrorRateThreshold(channelID int64, stat *metric.ChannelStat) {
	deliverAlert(AlertEvent{
		Type:      "error_rate_threshold",
		ChannelID: channelID,
		Message:   "channel error rate exceeded threshold",
		Timestamp: time.Now().Unix(),
	})
}

var (
	alertThrottleMu sync.Mutex
	lastAlertAt     = map[int64]time.Time{}
)

// alertThrottleWindow prevents repeated error-rate alerts for the same
// channel within this window.
const alertThrottleWindow = 5 * time.Minute

// CheckErrorRateThresholds scans every channel's rolling stats and fires
// an alert when the error rate crosses the configured threshold. The
// channel test task calls this after each probe cycle.
func CheckErrorRateThresholds() {
	threshold := setting.AlertErrorRateValue()
	if threshold <= 0 {
		return
	}
	for _, stat := range metric.TakeSnapshot().ChannelStats {
		if stat.Samples < 5 {
			continue // not enough data to trust the rate
		}
		errorRate := 1.0 - stat.SuccessRate
		if errorRate < threshold {
			continue
		}
		alertThrottleMu.Lock()
		last := lastAlertAt[stat.ChannelID]
		if time.Since(last) < alertThrottleWindow {
			alertThrottleMu.Unlock()
			continue
		}
		lastAlertAt[stat.ChannelID] = time.Now()
		alertThrottleMu.Unlock()
		NotifyErrorRateThreshold(stat.ChannelID, &stat)
	}
}

// deliverAlert sends the event to the log (always) and the webhook (when
// configured). The webhook delivery is best-effort: a failure is logged
// and never blocks the relay path.
func deliverAlert(ev AlertEvent) {
	// The log notifier is always on — it is the operator's floor.
	log.Printf("ALERT %s channel=%d: %s", ev.Type, ev.ChannelID, ev.Message)
	webhook := setting.AlertWebhookURLValue()
	if webhook == "" {
		return
	}
	go deliverWebhook(webhook, ev)
}

func deliverWebhook(webhook string, ev AlertEvent) {
	body, err := json.Marshal(ev)
	if err != nil {
		log.Printf("alert webhook marshal failed: %v", err)
		return
	}
	resp, err := alertHTTPClient.Post(webhook, "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("alert webhook delivery failed: %v", err)
		return
	}
	resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		log.Printf("alert webhook returned %d", resp.StatusCode)
	}
}

// ResetAlertThrottle clears the per-channel throttle map (tests).
func ResetAlertThrottle() {
	alertThrottleMu.Lock()
	lastAlertAt = map[int64]time.Time{}
	alertThrottleMu.Unlock()
}

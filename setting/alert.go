package setting

import (
	"encoding/json"
	"fmt"
	"sync"
)

// Alerting option keys (stored in the option table, hot-updatable):
//
//	alert_webhook_url  https://hooks.example.com/starhub  webhook that
//	    receives channel disable/enable and threshold alerts. Empty (the
//	    default) disables webhook delivery; the log notifier stays on.
//	alert_error_rate    0.5   per-channel error-rate threshold (0..1) that
//	    triggers an alert when the rolling window crosses it. 0 disables.
const (
	OptionKeyAlertWebhookURL = "alert_webhook_url"
	OptionKeyAlertErrorRate  = "alert_error_rate"
)

var (
	alertMu           sync.RWMutex
	alertWebhookURL   = ""
	alertErrorRate    = 0.0
)

// AlertOptionKeys lists the option keys owned by this file.
func AlertOptionKeys() []string {
	return []string{OptionKeyAlertWebhookURL, OptionKeyAlertErrorRate}
}

// IsAlertOptionKey reports whether the key is an alerting option.
func IsAlertOptionKey(key string) bool {
	switch key {
	case OptionKeyAlertWebhookURL, OptionKeyAlertErrorRate:
		return true
	}
	return false
}

// AlertWebhookURLValue returns the configured webhook URL ("" = disabled).
func AlertWebhookURLValue() string {
	alertMu.RLock()
	defer alertMu.RUnlock()
	return alertWebhookURL
}

// AlertErrorRateValue returns the per-channel error-rate alert threshold.
// 0 means the threshold check is disabled.
func AlertErrorRateValue() float64 {
	alertMu.RLock()
	defer alertMu.RUnlock()
	return alertErrorRate
}

// UpdateAlert applies an alerting option key.
func UpdateAlert(key, value string) error {
	switch key {
	case OptionKeyAlertWebhookURL:
		var v string
		if err := json.Unmarshal([]byte(value), &v); err != nil {
			// Accept a raw string too (the admin API may send an unquoted URL).
			v = value
		}
		alertMu.Lock()
		alertWebhookURL = v
		alertMu.Unlock()
		return nil
	case OptionKeyAlertErrorRate:
		var v float64
		if err := json.Unmarshal([]byte(value), &v); err != nil {
			return fmt.Errorf("invalid value for %s: %w", key, err)
		}
		if v < 0 || v != v || v > 1 {
			return fmt.Errorf("invalid value for %s: must be in [0,1]", key)
		}
		alertMu.Lock()
		alertErrorRate = v
		alertMu.Unlock()
		return nil
	default:
		return fmt.Errorf("unknown alert option key: %s", key)
	}
}

// SnapshotAlert returns the persisted representation of a key.
func SnapshotAlert(key string) (string, error) {
	alertMu.RLock()
	defer alertMu.RUnlock()
	switch key {
	case OptionKeyAlertWebhookURL:
		b, err := json.Marshal(alertWebhookURL)
		if err != nil {
			return "", err
		}
		return string(b), nil
	case OptionKeyAlertErrorRate:
		return fmt.Sprintf("%v", alertErrorRate), nil
	default:
		return "", fmt.Errorf("unknown alert option key: %s", key)
	}
}

// ResetAlertToDefaults restores the compiled-in alert defaults.
func ResetAlertToDefaults() {
	alertMu.Lock()
	defer alertMu.Unlock()
	alertWebhookURL = ""
	alertErrorRate = 0.0
}

package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type WatchDelivery struct {
	ID          string `json:"id"`
	Channel     string `json:"channel"`
	Rule        string `json:"rule"`
	Transition  string `json:"transition"`
	CreatedAt   int64  `json:"created_at"`
	NextAttempt int64  `json:"next_attempt"`
	Attempts    int    `json:"attempts"`
}
type WatchState struct {
	Conditions map[string]bool `json:"conditions"`
	Pending    []WatchDelivery `json:"pending"`
}

// WatchOnce is intentionally independent of the gateway process and its database.
func WatchOnce(ctx context.Context, c Config, target, statePath string, now time.Time) error {
	if ValidateEndpoint(target) != nil || target == "" || c.ObserverToken == "" {
		return errors.New("watchdog requires a valid health endpoint and observer token")
	}
	state := WatchState{Conditions: map[string]bool{}, Pending: []WatchDelivery{}}
	if raw, err := os.ReadFile(statePath); err == nil {
		if json.Unmarshal(raw, &state) != nil || state.Conditions == nil {
			return errors.New("invalid watchdog state")
		}
	} else if !os.IsNotExist(err) {
		return errors.New("watchdog state unavailable")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return errors.New("invalid health endpoint")
	}
	req.Header.Set("Authorization", "Bearer "+c.ObserverToken)
	response, err := HTTPClient().Do(req)
	gateway, database := false, false
	if err == nil {
		var health struct {
			Gateway  bool `json:"gateway"`
			Database bool `json:"database"`
		}
		err = json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&health)
		response.Body.Close()
		if err == nil && (response.StatusCode == 200 || response.StatusCode == 503) {
			gateway, database = health.Gateway, health.Database
		}
	}
	conditions := map[string]bool{"gateway_unavailable": !gateway}
	// An unreachable gateway cannot establish that a failed database recovered.
	if gateway {
		conditions["database_failure"] = !database
	}
	for rule, active := range conditions {
		prior, known := state.Conditions[rule]
		if active != prior && (known || active) {
			transition := "resolved"
			if active {
				transition = "firing"
			}
			for _, channel := range []string{"phone", "im"} {
				state.Pending = append(state.Pending, WatchDelivery{ID: NewID(), Rule: rule, Channel: channel, Transition: transition, CreatedAt: now.Unix()})
			}
		}
		state.Conditions[rule] = active
	}
	// Persist before sending; a restart retries the same delivery identity.
	if err := saveWatchState(statePath, state); err != nil {
		return err
	}
	pending := []WatchDelivery{}
	blocked := map[string]bool{}
	for _, delivery := range state.Pending {
		key := delivery.Rule + ":" + delivery.Channel
		endpoint := c.IMURL
		if delivery.Channel == "phone" {
			endpoint = c.PhoneURL
		}
		if endpoint == "" || ValidateEndpoint(endpoint) != nil || delivery.NextAttempt > now.Unix() || blocked[key] {
			pending = append(pending, delivery)
			blocked[key] = true
			continue
		}
		payload, _ := json.Marshal(map[string]any{"alert_id": delivery.ID, "rule": delivery.Rule, "severity": "critical", "transition": delivery.Transition, "occurred_at": delivery.CreatedAt, "source": "external_watchdog"})
		req, buildErr := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
		if buildErr != nil {
			pending = append(pending, delivery)
			blocked[key] = true
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", delivery.ID)
		if token := c.NotificationToken(delivery.Channel); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response, err := HTTPClient().Do(req)
		sent := false
		if err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
			response.Body.Close()
			sent = response.StatusCode >= 200 && response.StatusCode < 300
		}
		if !sent {
			delivery.Attempts++
			delivery.NextAttempt = now.Add(time.Duration(min(300, 5<<min(delivery.Attempts, 6))) * time.Second).Unix()
			pending = append(pending, delivery)
			blocked[key] = true
		}
	}
	state.Pending = pending
	return saveWatchState(statePath, state)
}

func saveWatchState(path string, state WatchState) error {
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".watch-state-")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	if _, err = temp.Write(raw); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

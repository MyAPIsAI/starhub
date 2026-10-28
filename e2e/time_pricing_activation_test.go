package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"starhub/model"
)

func TestActiveTimeRulesChangeNewRequestsButKeepInFlightSnapshot(t *testing.T) {
	env := setupRelayApp(t)
	response := doRequest(env.base, "PUT", "/api/option/pricing", env.session, map[string]any{"expected_version": 0, "options": []model.Option{{Key: "model_ratio", Value: `{"gpt-4o":2}`}, {Key: "completion_ratio", Value: `{"gpt-4o":2}`}}})
	if response.Status != 200 {
		t.Fatal("initial prices failed")
	}
	setRules := func(version int64, multiplier float64) {
		t.Helper()
		rules := json.RawMessage(fmt.Sprintf(`{"gpt-4o":{"timezone":"UTC","windows":[{"id":"all","label":"All","days":[0,1,2,3,4,5,6],"start":"00:00","end":"24:00","multiplier":%g}]}}`, multiplier))
		result := doRequest(env.base, "PUT", "/api/option/time-pricing", env.session, map[string]any{"expected_version": version, "rules": rules})
		if result.Status != 200 {
			t.Fatalf("time update: %d %s", result.Status, result.Body)
		}
	}
	setRules(2, 0.5)
	started, release := make(chan struct{}, 1), make(chan struct{})
	var releaseOnce sync.Once
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case started <- struct{}{}:
		default:
		}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"timed","choices":[{"index":0,"message":{"role":"assistant","content":"hello"}}],"usage":{"prompt_tokens":100,"completion_tokens":20}}`)
	}))
	defer upstream.Close()
	defer releaseOnce.Do(func() { close(release) })
	createChannelID(t, env, map[string]any{"name": "Timed local fixture", "key": upstreamKey, "base_url": upstream.URL, "models": "gpt-4o"})
	payload := map[string]any{"model": "gpt-4o", "messages": []map[string]string{{"role": "user", "content": "hello"}}, "max_tokens": 16}
	done := make(chan apiResult, 1)
	go func() { done <- doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, payload) }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("timed request did not reach local upstream")
	}
	setRules(3, 2)
	releaseOnce.Do(func() { close(release) })
	select {
	case result := <-done:
		if result.Status != 200 {
			t.Fatalf("timed request: %d %s", result.Status, result.Body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed request did not finish")
	}
	if result := doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, payload); result.Status != 200 {
		t.Fatal("next timed request failed")
	}
	logs, count, err := model.ListLogs(env.rootID, 1, 10, "")
	if err != nil || count != 2 || logs[0].Quota != 280 || logs[1].Quota != 70 {
		t.Fatalf("time adjustment was not frozen: %+v %v", logs, err)
	}
	root, err := model.GetUserByID(env.rootID)
	if err != nil || root.Quota != 100_000_000-350 || root.UsedQuota != 350 {
		t.Fatal("timed reservation/refund/log reconciliation failed")
	}
	var quote struct {
		Data struct {
			Model struct {
				Pricing struct {
					Input  float64 `json:"input_per_m"`
					Output float64 `json:"output_per_m"`
					Group  float64 `json:"group_ratio"`
					Time   struct {
						Multiplier float64   `json:"multiplier"`
						Window     string    `json:"window_id"`
						At         time.Time `json:"quoted_at"`
						Until      time.Time `json:"valid_until"`
					} `json:"time_pricing"`
				} `json:"pricing"`
			} `json:"model"`
		} `json:"data"`
	}
	result := doRequest(env.base, "GET", "/api/plaza/models/gpt-4o", env.session, nil)
	if err := json.Unmarshal([]byte(result.Body), &quote); err != nil || result.Status != 200 || quote.Data.Model.Pricing.Input != 4 || quote.Data.Model.Pricing.Output != 8 || quote.Data.Model.Pricing.Group != 1 || quote.Data.Model.Pricing.Time.Multiplier != 2 || quote.Data.Model.Pricing.Time.Window != "all" || !quote.Data.Model.Pricing.Time.Until.After(quote.Data.Model.Pricing.Time.At) {
		t.Fatalf("current time quote metadata: %d %s %v", result.Status, result.Body, err)
	}
}

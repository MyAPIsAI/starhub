package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"starhub/common"
	"starhub/model"
	"starhub/relay"
	"starhub/relay/dto"
)

var probeHTTPClient = &http.Client{Timeout: 30 * time.Second}

// upstreamUsageHTTPClient is a short-timeout client for upstream usage
// queries; it must not block the admin UI when the upstream is slow.
var upstreamUsageHTTPClient = &http.Client{Timeout: 10 * time.Second}

// disableKeywords are substrings (lower-cased) that prove an upstream channel
// is misconfigured; finding one in an error body is enough to auto-disable.
var disableKeywords = []string{
	"invalid_api_key",
	"invalid api key",
	"incorrect api key",
	"api key expired",
	"account_deactivated",
	"account deactivated",
	"authentication error",
}

// ShouldDisableChannel reports whether an upstream response (status code plus
// error body) proves the channel itself is broken and must be auto-disabled.
func ShouldDisableChannel(statusCode int, body string) bool {
	if statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden {
		return true
	}
	lower := strings.ToLower(body)
	for _, kw := range disableKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// ShouldEnableChannel reports whether a channel in the given status may be
// re-enabled automatically once a probe succeeds. Manually disabled channels
// always need an explicit admin action.
func ShouldEnableChannel(status int) bool {
	return status == common.ChannelStatusAutoDisabled
}

// DisableChannel marks a channel as auto-disabled and leaves a log trail for
// the administrator. An alert is fired so the operator is notified in real
// time (webhook when configured, log otherwise).
func DisableChannel(channelID int64, reason string) error {
	if err := model.UpdateChannelStatus(channelID, common.ChannelStatusAutoDisabled); err != nil {
		return err
	}
	log.Printf("channel %d auto-disabled: %s", channelID, reason)
	NotifyChannelDisabled(channelID, reason)
	return nil
}

// enableChannel restores an auto-disabled channel.
func enableChannel(channelID int64, reason string) error {
	if err := model.UpdateChannelStatus(channelID, common.ChannelStatusEnabled); err != nil {
		return err
	}
	log.Printf("channel %d auto-enabled: %s", channelID, reason)
	NotifyChannelEnabled(channelID, reason)
	return nil
}

// ProbeResult is the outcome of one health probe against a channel.
type ProbeResult struct {
	OK         bool   // true when the upstream answered with a success status
	ElapsedMs  int64  // round-trip latency in milliseconds
	Message    string // human-readable outcome, includes the upstream error snippet
	HTTPStatus int    // upstream status code, 0 when the request never got a response
	KeyIdx     int    // index of the upstream key used, -1 on single-key channels
}

// ProbeChannel sends a minimal chat request through the channel and reports
// the outcome. It performs no database writes; pair it with ApplyProbeResult
// to persist status transitions.
func ProbeChannel(ctx context.Context, ch *model.Channel, modelName string) ProbeResult {
	probe := &dto.ChatRequest{
		Model:     modelName,
		Messages:  []dto.ChatMessage{{Role: "user", Text: "ping"}},
		MaxTokens: 1,
	}
	adaptor, err := relay.GetChatAdaptor(ch.Type)
	if err != nil {
		return ProbeResult{Message: err.Error()}
	}
	body, err := adaptor.ConvertChatRequest(probe)
	if err != nil {
		return ProbeResult{Message: "build probe failed: " + err.Error()}
	}
	upstreamKey, keyIdx := ch.SelectKey()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		adaptor.ChatURL(ch.BaseURL, upstreamKey, modelName, false), bytes.NewReader(body))
	if err != nil {
		return ProbeResult{Message: "build request failed: " + err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(body))
	if err := adaptor.PrepareRequest(req, upstreamKey, body); err != nil {
		return ProbeResult{Message: "prepare request failed: " + err.Error()}
	}

	start := time.Now()
	resp, err := probeHTTPClient.Do(req)
	elapsed := time.Since(start).Milliseconds()
	if err != nil {
		return ProbeResult{KeyIdx: keyIdx, ElapsedMs: elapsed,
			Message: "request failed: " + err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return ProbeResult{
			KeyIdx:     keyIdx,
			ElapsedMs:  elapsed,
			HTTPStatus: resp.StatusCode,
			Message: fmt.Sprintf("upstream returned %d: %s",
				resp.StatusCode, strings.TrimSpace(string(body))),
		}
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return ProbeResult{OK: true, KeyIdx: keyIdx, ElapsedMs: elapsed,
		HTTPStatus: resp.StatusCode, Message: "ok"}
}

// ApplyProbeResult persists the effects of a probe: it records the measured
// response time and applies auto-enable / auto-disable transitions. On
// multi-key channels a failing probe only costs the probed key its job; the
// channel goes down when the last key dies. It returns the channel status
// after the probe.
func ApplyProbeResult(ch *model.Channel, res ProbeResult) int {
	if res.HTTPStatus > 0 {
		if err := model.UpdateChannelResponseTime(ch.ID, res.ElapsedMs); err != nil {
			log.Printf("update channel %d response time failed: %v", ch.ID, err)
		}
	}
	if res.OK {
		if ShouldEnableChannel(ch.Status) {
			if err := enableChannel(ch.ID, "probe succeeded"); err != nil {
				log.Printf("auto-enable channel %d failed: %v", ch.ID, err)
			} else {
				ch.Status = common.ChannelStatusEnabled
			}
		}
		return ch.Status
	}
	if ch.Status != common.ChannelStatusEnabled || !ShouldDisableChannel(res.HTTPStatus, res.Message) {
		return ch.Status
	}
	if !ch.IsMultiKey() {
		if err := DisableChannel(ch.ID, res.Message); err != nil {
			log.Printf("auto-disable channel %d failed: %v", ch.ID, err)
		} else {
			ch.Status = common.ChannelStatusAutoDisabled
		}
		return ch.Status
	}
	allDisabled, err := model.DisableChannelKey(ch.ID, res.KeyIdx)
	if err != nil {
		log.Printf("disable key %d of channel %d failed: %v", res.KeyIdx, ch.ID, err)
		return ch.Status
	}
	if allDisabled {
		if err := DisableChannel(ch.ID, res.Message); err != nil {
			log.Printf("auto-disable channel %d failed: %v", ch.ID, err)
		} else {
			ch.Status = common.ChannelStatusAutoDisabled
		}
	} else {
		log.Printf("channel %d: upstream key %d disabled by probe, other keys keep serving",
			ch.ID, res.KeyIdx)
	}
	return ch.Status
}

// UpstreamKeyUsage is the per-key usage snapshot fetched from the upstream
// new-api instance. It mirrors the fields returned by /api/usage/token/.
type UpstreamKeyUsage struct {
	KeyIndex       int    `json:"key_index"`
	KeyMasked      string `json:"key_masked"`
	Available      bool   `json:"available"`
	Error          string `json:"error,omitempty"`
	Name           string `json:"name,omitempty"`
	TotalUsed      int64  `json:"total_used"`
	TotalAvailable int64  `json:"total_available"`
	TotalGranted   int64  `json:"total_granted"`
	Unlimited      bool   `json:"unlimited"`
	ExpiresAt      int64  `json:"expires_at"`
	ModelLimits    any    `json:"model_limits,omitempty"`
	UserQuota      *struct {
		TotalAvailable int64 `json:"total_available"`
		TotalUsed      int64 `json:"total_used"`
	} `json:"user_quota,omitempty"`
}

// UpstreamUsageResult is the aggregated response for all keys of a channel.
type UpstreamUsageResult struct {
	ChannelID      int64              `json:"channel_id"`
	ChannelName    string             `json:"channel_name"`
	LocalUsedQuota int64              `json:"local_used_quota"`
	Keys           []UpstreamKeyUsage `json:"keys"`
	UpstreamTotal  int64              `json:"upstream_total_used"`
	Diff           int64              `json:"diff"`
	Supported      bool               `json:"supported"`
}

// FetchUpstreamTokenUsage queries the upstream new-api instance for each key
// of the channel and returns the per-key usage plus a reconciliation diff
// against the local Channel.UsedQuota. Multi-key channels are queried
// concurrently (max 5 parallel requests). Upstreams that are not new-api or
// do not expose /api/usage/token/ are reported as unsupported without error.
func FetchUpstreamTokenUsage(ctx context.Context, ch *model.Channel) (*UpstreamUsageResult, error) {
	result := &UpstreamUsageResult{
		ChannelID:      ch.ID,
		ChannelName:    ch.Name,
		LocalUsedQuota: ch.UsedQuota,
		Keys:           nil,
		Supported:      false,
	}
	if ch.BaseURL == "" {
		return result, nil
	}
	keys := ch.Keys()
	if len(keys) == 0 {
		return result, nil
	}
	base := strings.TrimRight(strings.TrimSpace(ch.BaseURL), "/")

	type queryResult struct {
		idx   int
		usage UpstreamKeyUsage
	}
	results := make([]UpstreamKeyUsage, len(keys))
	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup
	for i, key := range keys {
		wg.Add(1)
		go func(idx int, keyVal string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			usage := fetchOneUpstreamKey(ctx, base, keyVal, idx)
			results[idx] = usage
		}(i, key)
	}
	wg.Wait()

	result.Keys = results
	supported := false
	var upstreamTotal int64
	for i := range results {
		if results[i].Available {
			supported = true
			upstreamTotal += results[i].TotalUsed
		}
	}
	result.Supported = supported
	result.UpstreamTotal = upstreamTotal
	result.Diff = upstreamTotal - ch.UsedQuota
	return result, nil
}

func fetchOneUpstreamKey(ctx context.Context, base, key string, idx int) UpstreamKeyUsage {
	u := UpstreamKeyUsage{
		KeyIndex:  idx,
		KeyMasked: maskKey(key),
	}
	if key == "" {
		u.Error = "key is empty"
		return u
	}
	url := base + "/api/usage/token/"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		u.Error = "build request failed: " + err.Error()
		return u
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")
	resp, err := upstreamUsageHTTPClient.Do(req)
	if err != nil {
		u.Error = "request failed: " + err.Error()
		return u
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed {
		u.Error = "upstream does not support usage queries (not a new-api instance)"
		return u
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		u.Error = "upstream token invalid or disabled"
		return u
	}
	if resp.StatusCode >= http.StatusBadRequest {
		u.Error = fmt.Sprintf("upstream returned %d", resp.StatusCode)
		return u
	}
	var wrapper struct {
		Success bool            `json:"success"`
		Message string          `json:"message"`
		Code    bool            `json:"code"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		u.Error = "parse response failed: " + err.Error()
		return u
	}
	var data UpstreamKeyUsage
	raw := body
	if len(wrapper.Data) > 0 {
		raw = wrapper.Data
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		u.Error = "parse usage data failed: " + err.Error()
		return u
	}
	data.KeyIndex = idx
	data.KeyMasked = maskKey(key)
	data.Available = true
	return data
}

// maskKey returns a masked representation of a key for display: first 7 and
// last 4 characters visible, middle replaced with asterisks.
func maskKey(key string) string {
	if len(key) <= 11 {
		return strings.Repeat("*", len(key))
	}
	return key[:7] + strings.Repeat("*", len(key)-11) + key[len(key)-4:]
}

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
	"time"

	"starhub/common"
	"starhub/model"
	"starhub/relay"
)

var probeHTTPClient = &http.Client{Timeout: 30 * time.Second}

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
// the administrator (there is no notification backend yet, so stdout log is
// the operator surface).
func DisableChannel(channelID int64, reason string) error {
	if err := model.UpdateChannelStatus(channelID, common.ChannelStatusAutoDisabled); err != nil {
		return err
	}
	log.Printf("channel %d auto-disabled: %s", channelID, reason)
	return nil
}

// enableChannel restores an auto-disabled channel.
func enableChannel(channelID int64, reason string) error {
	if err := model.UpdateChannelStatus(channelID, common.ChannelStatusEnabled); err != nil {
		return err
	}
	log.Printf("channel %d auto-enabled: %s", channelID, reason)
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
	probe, err := json.Marshal(map[string]any{
		"model":      modelName,
		"messages":   []map[string]string{{"role": "user", "content": "ping"}},
		"max_tokens": 1,
	})
	if err != nil {
		return ProbeResult{Message: "build probe failed: " + err.Error()}
	}
	adaptor, err := relay.GetAdaptor(ch.Type)
	if err != nil {
		return ProbeResult{Message: err.Error()}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		adaptor.RequestURL(ch.BaseURL), bytes.NewReader(probe))
	if err != nil {
		return ProbeResult{Message: "build request failed: " + err.Error()}
	}
	upstreamKey, keyIdx := ch.SelectKey()
	adaptor.SetupRequestHeader(req, upstreamKey)

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

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"starhub/common"
	"starhub/model"
	"starhub/relay"
	"starhub/relay/channel"
	"starhub/relay/channel/ark"
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
	Diagnostic *ChannelModelsDiagnostic
	ImageURL   string // temporary preview URL; never persisted in channel health
	Code       string // controlled diagnostic code, separate from display text
	Scope      string // catalog checks do not establish inference health
	OK         bool   // true when the upstream answered with a success status
	ElapsedMs  int64  // round-trip latency in milliseconds
	Message    string // human-readable outcome, includes the upstream error snippet
	HTTPStatus int    // upstream status code, 0 when the request never got a response
	KeyIdx     int    // index of the upstream key used, -1 on single-key channels
}

// ProbeChannel checks Ark capabilities before selecting a catalog-only or chat probe.
// It performs no database writes; pair it with ApplyProbeResult for inference health updates.
func ProbeChannel(ctx context.Context, ch *model.Channel, modelName string) ProbeResult {
	upstreamKey, keyIdx := ch.SelectKey()
	if ch.Type == channel.ChannelTypeArk {
		if upstreamKey == "" {
			return ProbeResult{Scope: "catalog", KeyIdx: keyIdx, Message: "models_key_required"}
		}
		if ch.ModelCapabilities[modelName] == "VideoGeneration" || ark.IsVideoModel(modelName) {
			return probeArkVideoAPI(ctx, ch, upstreamKey, keyIdx)
		}
		catalog, domain := probeArkCatalog(ctx, ch, modelName, upstreamKey, keyIdx)
		if !catalog.OK || (domain != "LLM" && domain != "VLM") {
			return catalog
		}
	}
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
	if res.Scope == "catalog" || res.Scope == "video_api" {
		return ch.Status
	}
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
	Code           string `json:"code,omitempty"`
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
	Status             string             `json:"status"`
	Code               string             `json:"code,omitempty"`
	LocalUsedQuotaNano string             `json:"local_used_quota_nano"`
	UpstreamTotalNano  string             `json:"upstream_total_used_nano,omitempty"`
	DiffNano           string             `json:"diff_nano,omitempty"`
	ChannelID          int64              `json:"channel_id"`
	ChannelName        string             `json:"channel_name"`
	LocalUsedQuota     int64              `json:"local_used_quota"`
	Keys               []UpstreamKeyUsage `json:"keys"`
	UpstreamTotal      int64              `json:"upstream_total_used"`
	Diff               int64              `json:"diff"`
	Supported          bool               `json:"supported"`
}

// FetchUpstreamTokenUsage queries the upstream new-api instance for each key
// of the channel and returns the per-key usage plus a reconciliation diff
// against the local Channel.UsedQuota. Multi-key channels are queried
// concurrently (max 5 parallel requests). Unsupported endpoints, failed queries
// and partial snapshots have distinct statuses. Differences require all keys.
func FetchUpstreamTokenUsage(ctx context.Context, ch *model.Channel) (*UpstreamUsageResult, error) {
	result := &UpstreamUsageResult{
		ChannelID:      ch.ID,
		ChannelName:    ch.Name,
		LocalUsedQuota: ch.UsedQuota, LocalUsedQuotaNano: common.QuotaMoney(ch.UsedQuota, ch.UsedQuotaRemainderNano).String(),
		Keys:      []UpstreamKeyUsage{},
		Supported: false, Status: "failed",
	}
	if ch.Type == channel.ChannelTypeArk {
		result.Status = "unsupported"
		result.Code = "ark_usage_not_supported"
		return result, nil
	}
	if ch.BaseURL == "" {
		result.Code = "usage_base_url_required"
		return result, nil
	}
	keys := ch.Keys()
	if len(keys) == 0 {
		result.Code = "usage_key_required"
		return result, nil
	}
	base := strings.TrimRight(strings.TrimSpace(ch.BaseURL), "/")

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
	available, unsupported := 0, 0
	upstreamTotal := new(big.Int)
	for i := range results {
		if results[i].Available {
			available++
			upstreamTotal.Add(upstreamTotal, big.NewInt(results[i].TotalUsed))
		}
		if results[i].Code == "usage_not_supported" {
			unsupported++
		}
	}
	result.Supported = available > 0
	switch {
	case available == len(results):
		result.Status = "ok"
	case available > 0:
		result.Status, result.Code = "partial", "usage_incomplete"
	case unsupported == len(results):
		result.Status, result.Code = "unsupported", "usage_not_supported"
	default:
		result.Code = "usage_query_failed"
	}
	result.UpstreamTotal = boundedUsageInteger(upstreamTotal)
	nano := new(big.Int).Mul(upstreamTotal, big.NewInt(common.NanoPerLegacyQuota))
	if result.Supported {
		result.UpstreamTotalNano = nano.String()
	}
	if result.Status == "ok" {
		difference := new(big.Int).Sub(nano, common.QuotaMoney(ch.UsedQuota, ch.UsedQuotaRemainderNano))
		result.Diff = boundedUsageInteger(new(big.Int).Quo(difference, big.NewInt(common.NanoPerLegacyQuota)))
		result.DiffNano = difference.String()
	}
	return result, nil
}

func fetchOneUpstreamKey(ctx context.Context, base, key string, idx int) UpstreamKeyUsage {
	u := UpstreamKeyUsage{
		KeyIndex:  idx,
		KeyMasked: maskKey(key),
	}
	fail := func(code, message string) UpstreamKeyUsage {
		u.Code, u.Error = code, message
		return u
	}
	if key == "" {
		return fail("usage_key_required", "key is empty")
	}
	url := base + "/api/usage/token/"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fail("usage_invalid_url", "invalid upstream URL")
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")
	resp, err := upstreamUsageHTTPClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fail("usage_timeout", "upstream usage query timed out")
		}
		return fail("usage_network_error", "upstream usage query could not connect")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed {
		return fail("usage_not_supported", "upstream does not support usage queries")
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fail("usage_auth_failed", "upstream token invalid, disabled or not authorized")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fail("usage_http_error", fmt.Sprintf("upstream returned %d", resp.StatusCode))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil || len(body) > 1<<20 {
		return fail("usage_invalid_response", "upstream usage response could not be read")
	}
	var wrapper struct {
		Success *bool           `json:"success"`
		Code    json.RawMessage `json:"code"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return fail("usage_invalid_response", "upstream usage response is not valid JSON")
	}
	if (wrapper.Success != nil && !*wrapper.Success) || string(wrapper.Code) == "false" {
		return fail("usage_query_rejected", "upstream rejected the usage query")
	}
	var data struct {
		UpstreamKeyUsage
		Used           *int64 `json:"total_used"`
		Remaining      *int64 `json:"total_available"`
		IsUnlimited    *bool  `json:"unlimited"`
		UnlimitedQuota *bool  `json:"unlimited_quota"`
	}
	raw := body
	if len(wrapper.Data) > 0 {
		raw = wrapper.Data
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return fail("usage_invalid_response", "upstream usage data has an invalid format")
	}
	if data.Used == nil || data.Remaining == nil || *data.Used < 0 || data.TotalGranted < 0 {
		return fail("usage_invalid_response", "upstream usage amounts are missing or invalid")
	}
	data.TotalUsed, data.TotalAvailable = *data.Used, *data.Remaining
	if data.IsUnlimited != nil {
		data.Unlimited = *data.IsUnlimited
	}
	if data.UnlimitedQuota != nil {
		if data.IsUnlimited != nil && *data.IsUnlimited != *data.UnlimitedQuota {
			return fail("usage_invalid_response", "upstream unlimited quota flags conflict")
		}
		data.Unlimited = *data.UnlimitedQuota
	}
	data.KeyIndex = idx
	data.KeyMasked = maskKey(key)
	data.Available = true
	data.Code, data.Error = "", ""
	return data.UpstreamKeyUsage
}

// maskKey returns a masked representation of a key for display: first 7 and
// last 4 characters visible, with a fixed-length masked middle.
func maskKey(key string) string {
	if len(key) <= 11 {
		return "********"
	}
	return key[:7] + "********" + key[len(key)-4:]
}

func boundedUsageInteger(value *big.Int) int64 {
	if value.IsInt64() {
		return value.Int64()
	}
	if value.Sign() < 0 {
		return math.MinInt64
	}
	return math.MaxInt64
}

func (u UpstreamKeyUsage) MarshalJSON() ([]byte, error) {
	type alias UpstreamKeyUsage
	if !u.Available {
		return json.Marshal(alias(u))
	}
	return json.Marshal(struct {
		alias
		UsedNano      string `json:"total_used_nano"`
		AvailableNano string `json:"total_available_nano"`
		GrantedNano   string `json:"total_granted_nano"`
	}{alias(u), common.QuotaMoney(u.TotalUsed, 0).String(), common.QuotaMoney(u.TotalAvailable, 0).String(), common.QuotaMoney(u.TotalGranted, 0).String()})
}

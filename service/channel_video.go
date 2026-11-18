package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"starhub/model"
	"starhub/relay/channel/ark"
)

var channelVideoClient = &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

// probeArkVideoAPI checks read-only task access without requiring the model catalog.
func probeArkVideoAPI(ctx context.Context, ch *model.Channel, key string, index int) ProbeResult {
	start := time.Now()
	result := ProbeResult{Scope: "video_api", KeyIdx: index, Code: "upstream_video_failed"}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ark.APIBaseURL(ch.BaseURL)+"/contents/generations/tasks?page_size=1", nil)
	if err != nil {
		return result
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(key))
	req.Header.Set("Accept", "application/json")
	resp, err := channelVideoClient.Do(req)
	result.ElapsedMs = time.Since(start).Milliseconds()
	if err != nil {
		result.Code = "upstream_video_network_error"
		return result
	}
	defer resp.Body.Close()
	result.HTTPStatus = resp.StatusCode
	if resp.StatusCode != http.StatusOK {
		failure := channelUpstreamError(resp, req, key)
		result.Code = strings.Replace(failure.Code, "upstream_models_", "upstream_video_", 1)
		result.Diagnostic = failure.Diagnostic
		return result
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	var envelope struct {
		Items *[]json.RawMessage `json:"items"`
		Total *int               `json:"total"`
	}
	if err != nil || len(body) > 1<<20 || json.Unmarshal(body, &envelope) != nil || envelope.Items == nil || envelope.Total == nil || *envelope.Total < 0 {
		result.Code = "upstream_video_invalid_response"
		return result
	}
	result.OK, result.Code, result.Message = true, "ark_video_api_accessible", "Video task API accessible; generation was not tested"
	return result
}

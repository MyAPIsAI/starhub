package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"starhub/model"
	"starhub/relay/channel"
	"starhub/relay/channel/ark"
	"starhub/relay/dto"
)

func probeArkCatalog(ctx context.Context, ch *model.Channel, name, key string, keyIdx int) (ProbeResult, string) {
	start := time.Now()
	result := ProbeResult{Scope: "catalog", KeyIdx: keyIdx}
	catalog, err := FetchArkChannelModelCatalog(ctx, ch.BaseURL, key, "bearer")
	result.ElapsedMs = time.Since(start).Milliseconds()
	if err != nil {
		result.Message = "upstream_models_failed"
		var failure *ChannelModelsError
		if errors.As(err, &failure) {
			result.Message = failure.Code
		}
		return result, ""
	}
	for _, entry := range catalog {
		if entry.ID == name {
			result.OK, result.HTTPStatus = true, http.StatusOK
			result.Message = "Model catalog accessible; inference was not tested"
			return result, entry.Domain
		}
	}
	result.Message = "Model ID not in catalog; use an exact version ID. Endpoint IDs require an explicit inference test"
	return result, ""
}

// ProbeChannelImage sends one explicitly requested generation; automatic checks never call it.
func ProbeChannelImage(ctx context.Context, ch *model.Channel, name string) (result ProbeResult) {
	result.Scope = "image"
	if ch.Type != channel.ChannelTypeArk {
		result.Message = "image_probe_unsupported"
		return
	}
	key, keyIdx := ch.SelectKey()
	result.KeyIdx = keyIdx
	if key == "" {
		result.Message = "models_key_required"
		return
	}
	endpoint, err := (ark.Adaptor{}).ModeURL(ch.BaseURL, channel.RelayModeImageGeneration)
	if err != nil {
		result.Message = "image_probe_unsupported"
		return
	}
	body, _ := json.Marshal(map[string]string{"model": name, "prompt": "A blue circle on a white background", "size": "2048x2048", "response_format": "url"})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		result.Message = "invalid_image_probe_url"
		return
	}
	req.Header.Set("Content-Type", "application/json")
	_ = (ark.Adaptor{}).PrepareRequest(req, key, body)
	start := time.Now()
	defer func() { result.ElapsedMs = time.Since(start).Milliseconds() }()
	client := &http.Client{Timeout: 90 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		result.Message = "Image test request failed; upstream execution may be uncertain"
		return
	}
	defer resp.Body.Close()
	result.HTTPStatus = resp.StatusCode
	if resp.StatusCode != http.StatusOK {
		result.Message = "Upstream image generation failed"
		return
	}
	const maxBytes = 2 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil || len(data) > maxBytes {
		result.Message = "Invalid image test response"
		return
	}
	urls, b64s, _, err := dto.WalkImageResponse(data, nil)
	result.OK = err == nil && (urls > 0 || b64s > 0)
	result.Message = "Image generation returned no valid image"
	if result.OK {
		result.Message = "Image generated successfully"
	}
	return
}

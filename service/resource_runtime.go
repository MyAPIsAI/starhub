package service

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"starhub/model"
)

// Platform probes never use customer credentials, balances, logs or attempt counters.
func probeResource(ctx context.Context, probe model.ResourceProbe) bool {
	ch, err := model.GetChannelByID(probe.ChannelID)
	if err != nil || ch.Status != 1 {
		return false
	}
	r := probe.Resource
	if slices.Contains(r.Capabilities, "text") || slices.Contains(r.Capabilities, "code") {
		return ProbeChannel(ctx, ch, r.UpstreamModel).OK
	}
	key, _ := ch.SelectKey()
	endpoint := strings.TrimRight(ch.BaseURL, "/") + "/v1/models"
	if ch.Type == 25 {
		endpoint = strings.TrimRight(ch.BaseURL, "/") + "/v1beta/models?key=" + url.QueryEscape(key)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false
	}
	if ch.Type == 14 {
		req.Header.Set("x-api-key", key)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else if ch.Type != 25 {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func WatchResourceRuntime(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		if err := model.LocalResources.Publish(ctx, time.Now()); err != nil {
			log.Printf("resource status report failed: %v", err)
		}
		for _, probe := range model.LocalResources.DueProbes(time.Now()) {
			go func(probe model.ResourceProbe) {
				probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				defer cancel()
				ok := probeResource(probeCtx, probe)
				model.LocalResources.FinishProbe(probe.Resource, time.Now(), ok)
			}(probe)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

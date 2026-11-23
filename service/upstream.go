package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"starhub/common"
	"starhub/internal/upstream"
	"starhub/model"
	"starhub/relay"
)

// Recovery probes use the failing credential, never another key in the channel.
func probeProtected(ctx context.Context, s upstream.State, p upstream.Policy) (string, upstream.Verdict, int) {
	unknown := upstream.Verdict{Class: "probe_not_applicable"}
	ch, err := model.GetChannelByID(s.ChannelID)
	if err != nil || ch.Status != common.ChannelStatusEnabled || !model.UpstreamStateCurrent(ch, s) {
		return "unknown", unknown, 0
	}
	path := protectionProbePath(s)
	if path == "" {
		return "unknown", unknown, 0
	}
	allowed, err := model.UpstreamProbeAllowed(ch, s.Model, path)
	if err != nil || !allowed {
		return "unknown", unknown, 0
	}
	key, found := "", false
	keys := ch.Keys()
	statuses := ch.KeyStatuses()
	if len(keys) == 0 && s.Credential == upstream.CredentialID("") {
		found = true
	}
	for index, candidate := range keys {
		if upstream.CredentialID(candidate) == s.Credential && statuses[index] == common.StatusEnabled {
			key, found = candidate, true
			break
		}
	}
	if !found {
		return "unknown", unknown, 0
	}
	admitted, err := upstream.ProbeAdmitted(model.DB.WithContext(ctx), s)
	if err != nil || !admitted {
		return "unknown", upstream.Verdict{Class: "probe_blocked"}, 0
	}
	adaptor, err := relay.GetChatAdaptor(ch.Type)
	if err != nil {
		return "unknown", unknown, 0
	}
	if s.Capability == "realtime" {
		url, err := adaptor.RealtimeURL(ch.BaseURL, s.Model)
		if err != nil {
			return "unknown", unknown, 0
		}
		conn, err := relay.WSDialContext(ctx, url, http.Header{"Authorization": []string{"Bearer " + key}, "OpenAI-Beta": []string{"realtime=v1"}})
		if err != nil {
			var handshake *relay.WSHandshakeError
			if errors.As(err, &handshake) {
				v := upstream.HTTP(handshake.Status, handshake.Body, handshake.RetryAfter, p)
				if v.Scope == "" {
					return "unknown", v, handshake.Status
				}
				return "failure", v, handshake.Status
			}
			if errors.Is(ctx.Err(), context.Canceled) {
				return "unknown", upstream.Verdict{Class: "probe_cancelled"}, 0
			}
			return "failure", upstream.Verdict{Class: "transport_failure"}, 0
		}
		conn.Close()
		return "success", upstream.Verdict{Class: "probe_completed"}, 101
	}
	body, url, err := protectionProbeRequest(adaptor, ch, s, key)
	if err != nil {
		return "unknown", unknown, 0
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "unknown", unknown, 0
	}
	req.Header.Set("Content-Type", "application/json")
	if err := adaptor.PrepareRequest(req, key, body); err != nil {
		return "unknown", unknown, 0
	}
	client := &http.Client{Timeout: time.Duration(p.ProbeTimeout) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return "unknown", upstream.Verdict{Class: "probe_cancelled"}, 0
		}
		return "failure", upstream.Verdict{Class: "transport_failure"}, 0
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		v := upstream.HTTP(resp.StatusCode, body, resp.Header.Get("Retry-After"), p)
		if v.Scope == "" {
			return "unknown", v, resp.StatusCode
		}
		return "failure", v, resp.StatusCode
	}
	if !protectionProbeComplete(adaptor, s, resp) {
		if errors.Is(ctx.Err(), context.Canceled) {
			return "unknown", upstream.Verdict{Class: "probe_cancelled"}, resp.StatusCode
		}
		return "failure", upstream.Verdict{Class: "protocol_failure"}, resp.StatusCode
	}
	return "success", upstream.Verdict{Class: "probe_completed"}, resp.StatusCode
}

func ProbeProtectedResource(ctx context.Context, id string) error {
	p := upstream.CurrentPolicy()
	if !p.Enabled {
		return upstream.ErrBlocked
	}
	if err := model.PrepareUpstreamProbe(ctx, id); err != nil {
		return err
	}
	lease, err := upstream.Claim(model.DB.WithContext(ctx), id, p)
	if err != nil {
		return err
	}
	if lease == nil {
		return upstream.ErrBlocked
	}
	probeCtx, cancel := context.WithTimeout(ctx, time.Duration(p.ProbeTimeout)*time.Second)
	outcome, verdict, status := probeProtected(probeCtx, *lease, p)
	cancel()
	// Recheck the endpoint after the network call; old probes cannot recover new credentials.
	ch, err := model.GetChannelByID(lease.ChannelID)
	if err != nil || ch.Status != common.ChannelStatusEnabled || !model.UpstreamStateCurrent(ch, *lease) {
		outcome = "unknown"
	} else if allowed, err := model.UpstreamProbeAllowed(ch, lease.Model, protectionProbePath(*lease)); err != nil || !allowed {
		outcome = "unknown"
	}
	saveCtx, saveCancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer saveCancel()
	err = upstream.CompleteProbe(model.DB.WithContext(saveCtx), *lease, outcome, verdict, status, p)
	if err != nil {
		upstream.PersistenceFailed()
	}
	return err
}

func RunUpstreamProtection(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	lastCleanup := time.Time{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		p := upstream.CurrentPolicy()
		if time.Since(lastCleanup) > time.Hour {
			if err := upstream.Cleanup(ctx, model.DB); err != nil {
				log.Printf("upstream event cleanup failed: %v", err)
			}
			if err := model.CleanupUpstreamStates(ctx); err != nil {
				log.Printf("upstream state cleanup failed: %v", err)
			}
			lastCleanup = time.Now()
		}
		if !p.Enabled {
			continue
		}
		states, err := model.CurrentUpstreamStates(ctx, true, p.ProbeConcurrency*16)
		if err != nil {
			log.Printf("upstream probe candidates failed: %v", err)
			continue
		}
		var wg sync.WaitGroup
		workers := make(chan struct{}, p.ProbeConcurrency)
		for _, s := range states {
			workers <- struct{}{}
			wg.Add(1)
			go func(id string) {
				defer wg.Done()
				defer func() { <-workers }()
				if err := ProbeProtectedResource(ctx, id); err != nil && !errors.Is(err, upstream.ErrBlocked) {
					log.Printf("upstream probe failed: %v", err)
				}
			}(s.ID)
		}
		wg.Wait()
	}
}

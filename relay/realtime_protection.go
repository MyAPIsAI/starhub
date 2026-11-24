package relay

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"starhub/internal/upstream"
	"starhub/model"
)

// A handshake or a usage frame alone does not prove successful model execution.
func realtimeBusinessResult(data []byte, policy upstream.Policy) (upstream.Verdict, int, bool) {
	type providerError struct {
		Code string `json:"code"`
		Type string `json:"type"`
	}
	var frame struct {
		Type     string         `json:"type"`
		Error    *providerError `json:"error"`
		Response *struct {
			Status        string `json:"status"`
			StatusDetails *struct {
				Error *providerError `json:"error"`
			} `json:"status_details"`
		} `json:"response"`
	}
	if json.Unmarshal(data, &frame) != nil {
		return upstream.Verdict{}, 0, false
	}
	provider := frame.Error
	if frame.Type == "response.done" && frame.Response != nil {
		if frame.Response.StatusDetails != nil {
			provider = frame.Response.StatusDetails.Error
		}
		if frame.Response.Status == "completed" && provider == nil {
			return upstream.Verdict{Class: "success"}, 200, true
		}
	} else if frame.Type != "error" {
		return upstream.Verdict{}, 0, false
	}
	if provider == nil {
		return upstream.Verdict{Class: "business_incomplete"}, 0, true
	}
	status := 400
	code := strings.ToLower(provider.Code + " " + provider.Type)
	switch {
	case strings.Contains(code, "rate_limit"), strings.Contains(code, "insufficient_quota"):
		status = 429
	case strings.Contains(code, "authentication"), strings.Contains(code, "invalid_api_key"):
		status = 401
	case strings.Contains(code, "model_not_found"), strings.Contains(code, "model_not_supported"):
		status = 404
	case strings.Contains(code, "server_error"), strings.Contains(code, "internal_error"):
		status = 500
	}
	body, _ := json.Marshal(map[string]any{"error": provider})
	return upstream.HTTP(status, body, "", policy), status, true
}

func routingResultClass(v upstream.Verdict) string {
	switch v.Class {
	case "credential_rejected":
		return "credential_error"
	case "model_unavailable":
		return "model_not_found"
	case "transport_failure", "upstream_failure", "protocol_failure":
		return "upstream_unavailable"
	case "request_rejected":
		return "customer_error"
	case "client_disconnected":
		return "cancelled"
	case "success", "rate_limited":
		return v.Class
	default:
		return "unknown"
	}
}

// realtimeProtectionSession tracks the admission snapshot for one live session.
// Each business result reports the transitions it actually committed; only
// those owned changes advance the snapshot. A scope whose row changed behind
// the session is invalidated so a later success cannot absorb it.
type realtimeProtectionSession struct {
	mu      sync.Mutex
	attempt upstream.Attempt
	invalid map[string]bool
}

func newRealtimeProtectionSession(captured *upstream.Attempt) *realtimeProtectionSession {
	session := &realtimeProtectionSession{invalid: map[string]bool{}}
	if captured == nil {
		return session
	}
	session.attempt = *captured
	session.attempt.Versions = cloneVersions(captured.Versions)
	session.attempt.Failures = cloneFailures(captured.Failures)
	session.attempt.Protected = cloneVersions(captured.Protected)
	session.attempt.Recovered = cloneVersions(captured.Recovered)
	return session
}

func cloneVersions(source map[string]uint64) map[string]uint64 {
	target := make(map[string]uint64, len(source))
	for key, value := range source {
		target[key] = value
	}
	return target
}

func cloneFailures(source map[string]int) map[string]int {
	target := make(map[string]int, len(source))
	for key, value := range source {
		target[key] = value
	}
	return target
}

// snapshot returns an isolated attempt so Record cannot mutate live state.
func (s *realtimeProtectionSession) snapshot() upstream.Attempt {
	attempt := s.attempt
	attempt.Versions = cloneVersions(s.attempt.Versions)
	attempt.Failures = cloneFailures(s.attempt.Failures)
	attempt.Protected = cloneVersions(s.attempt.Protected)
	attempt.Recovered = cloneVersions(s.attempt.Recovered)
	return attempt
}

// apply follows only transitions this session committed.
func (s *realtimeProtectionSession) apply(committed upstream.Committed) {
	for id := range committed.Stale {
		s.invalid[id] = true
		delete(s.attempt.Versions, id)
		delete(s.attempt.Failures, id)
	}
	for id, version := range committed.Versions {
		if s.invalid[id] {
			continue
		}
		s.attempt.Versions[id] = version
	}
	for id, failures := range committed.Failures {
		if s.invalid[id] {
			continue
		}
		s.attempt.Failures[id] = failures
	}
}

func recordRealtimeBusiness(ctx context.Context, session *realtimeProtectionSession, route *model.RoutingAttempt, requestID string, verdict upstream.Verdict, status int, policy upstream.Policy) {
	if session == nil {
		return
	}
	session.mu.Lock()
	attempt := session.snapshot()
	session.mu.Unlock()
	attempt.ID = uuid.NewString()
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	committed, err := upstream.RecordCommitted(model.DB.WithContext(saveCtx), &attempt, verdict, upstream.Event{RequestID: requestID, Sent: true, Status: status, Decision: "response_started"}, policy)
	if err != nil {
		log.Printf("realtime business result persistence failed: %v", err)
	} else {
		session.mu.Lock()
		session.apply(committed)
		session.mu.Unlock()
	}
	if verdict.Class == "success" {
		if route != nil && !route.OrdinaryFallback {
			if err := model.RecordRoutingOutcome(route, true, status, 0, time.Now()); err != nil {
				log.Printf("realtime routing success persistence failed: %v", err)
			}
		}
		if err := model.RecoverFallbackRouting(route, &attempt, requestID, time.Now()); err != nil {
			log.Printf("realtime fallback recovery failed: %v", err)
		}
	} else if verdict.Scope != "" {
		if err := model.RecordRoutingOutcome(route, false, status, 0, time.Now(), true); err != nil {
			log.Printf("realtime routing failure persistence failed: %v", err)
		}
	}
}

func finishRealtimeRouting(route *model.RoutingAttempt, requestID string, verdict upstream.Verdict, status int) {
	if verdict.Scope != "" {
		if err := model.RecordRoutingOutcome(route, false, status, 0, time.Now()); err != nil {
			log.Printf("realtime routing failure persistence failed: %v", err)
		}
	}
	if err := model.EndRoutingAttempt(route, requestID, routingResultClass(verdict), status, time.Now()); err != nil {
		log.Printf("realtime routing event failed: %v", err)
	}
}

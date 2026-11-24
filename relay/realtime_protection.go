package relay

import (
	"context"
	"encoding/json"
	"log"
	"strings"
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

func recordRealtimeBusiness(ctx context.Context, captured *upstream.Attempt, route *model.RoutingAttempt, requestID string, verdict upstream.Verdict, status int, policy upstream.Policy) {
	// Keep the original captured versions: a later result must not erase newer failures.
	attempt := *captured
	attempt.ID = uuid.NewString()
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	if err := upstream.Record(model.DB.WithContext(saveCtx), &attempt, verdict, upstream.Event{RequestID: requestID, Sent: true, Status: status, Decision: "response_started"}, policy); err != nil {
		log.Printf("realtime business result persistence failed: %v", err)
	}
	if verdict.Class == "success" {
		if err := model.RecoverFallbackRouting(route, &attempt, requestID, time.Now()); err != nil {
			log.Printf("realtime fallback recovery failed: %v", err)
		}
	} else if verdict.Scope != "" {
		if err := model.RecordRoutingOutcome(route, false, status, 0, time.Now()); err != nil {
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

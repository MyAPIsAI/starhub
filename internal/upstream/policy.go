// Package upstream owns retry classification and durable resource protection.
package upstream

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const OptionKey = "upstream_protection"

type Policy struct {
	Enabled          bool `json:"enabled"`
	Retry            bool `json:"retry"`
	MaxAttempts      int  `json:"max_attempts"`
	RetryWindow      int  `json:"retry_window_seconds"`
	Failures         int  `json:"failure_threshold"`
	OpenSeconds      int  `json:"open_seconds"`
	MaxOpenSeconds   int  `json:"max_open_seconds"`
	RateSeconds      int  `json:"rate_limit_seconds"`
	IsolationSeconds int  `json:"isolation_seconds"`
	ProbeConcurrency int  `json:"probe_concurrency"`
	ProbeTimeout     int  `json:"probe_timeout_seconds"`
}

func DefaultPolicy() Policy { return Policy{true, true, 4, 60, 5, 30, 300, 30, 900, 4, 30} }

var policyMu sync.RWMutex
var currentPolicy = DefaultPolicy()

func CurrentPolicy() Policy { policyMu.RLock(); defer policyMu.RUnlock(); return currentPolicy }
func NormalizePolicy(value string) (string, error) {
	if !strings.HasPrefix(strings.TrimSpace(value), "{") {
		return "", errors.New("expected a policy object")
	}
	p := DefaultPolicy()
	d := json.NewDecoder(strings.NewReader(value))
	d.DisallowUnknownFields()
	if err := d.Decode(&p); err != nil {
		return "", err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return "", errors.New("expected a single policy object")
	}
	if p.MaxAttempts < 1 || p.MaxAttempts > 10 || p.RetryWindow < 1 || p.RetryWindow > 600 || p.Failures < 1 || p.Failures > 100 || p.OpenSeconds < 1 || p.MaxOpenSeconds < p.OpenSeconds || p.MaxOpenSeconds > 3600 || p.RateSeconds < 1 || p.RateSeconds > 86400 || p.IsolationSeconds < 60 || p.IsolationSeconds > 86400 || p.ProbeConcurrency < 1 || p.ProbeConcurrency > 16 || p.ProbeTimeout < 1 || p.ProbeTimeout > 120 {
		return "", errors.New("invalid upstream protection policy")
	}
	b, err := json.Marshal(p)
	return string(b), err
}
func ApplyPolicy(value string) error {
	normalized, err := NormalizePolicy(value)
	if err != nil {
		return err
	}
	var p Policy
	_ = json.Unmarshal([]byte(normalized), &p)
	policyMu.Lock()
	currentPolicy = p
	policyMu.Unlock()
	return nil
}
func PolicyJSON() string { b, _ := json.Marshal(CurrentPolicy()); return string(b) }

type Verdict struct {
	Class     string `json:"class"`
	Scope     string `json:"scope"`
	Retry     bool   `json:"retry"`
	Immediate bool   `json:"-"`
	Delay     int    `json:"-"`
}

// HTTP classifies provider errors before any response is committed downstream.
func HTTP(status int, body []byte, retryAfter string, p Policy) Verdict {
	var envelope struct {
		Error struct {
			Code string `json:"code"`
			Type string `json:"type"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &envelope)
	code := strings.ToLower(envelope.Error.Code + " " + envelope.Error.Type)
	contains := func(values ...string) bool {
		for _, value := range values {
			if strings.Contains(code, value) {
				return true
			}
		}
		return false
	}
	if contains("content_policy", "content_filter", "safety", "moderation") {
		return Verdict{Class: "request_rejected"}
	}
	if (status == 402 || status == 403 || status == 429) && contains("insufficient_quota", "insufficient_balance", "credit_balance", "billing_hard_limit", "quota_exhausted") {
		return Verdict{"quota_exhausted", "credential", true, true, p.IsolationSeconds}
	}
	if status == 401 || status == 403 && contains("invalid_api_key", "invalid_token", "authentication_error", "credential_revoked", "key_expired") {
		return Verdict{"credential_rejected", "credential", true, true, p.IsolationSeconds}
	}
	if (status == 403 || status == 404) && contains("model_not_found", "model_not_supported", "model_access_denied", "model_permission_denied") {
		return Verdict{"model_unavailable", "credential_model", true, true, p.IsolationSeconds}
	}
	if status == 429 {
		return Verdict{"rate_limited", "credential_model", true, true, retryDelay(retryAfter, p.RateSeconds)}
	}
	if status == 408 || status >= 500 {
		return Verdict{Class: "upstream_failure", Scope: "interface", Retry: true}
	}
	return Verdict{Class: "request_rejected"}
}

func retryDelay(value string, fallback int) int {
	value = strings.TrimSpace(value)
	if len(value) > 128 {
		return fallback
	}
	if seconds, err := strconv.ParseUint(value, 10, 32); err == nil {
		return max(1, min(86400, int(seconds)))
	}
	if deadline, err := http.ParseTime(value); err == nil && deadline.After(time.Now()) {
		return max(1, min(86400, int(time.Until(deadline).Seconds())+1))
	}
	return fallback
}

type RetryInput struct {
	Cancelled, Committed, Settled, Strict, Skip, Unsafe bool
	Attempts                                            int
	Elapsed                                             time.Duration
}

func RetryReason(p Policy, verdict Verdict, in RetryInput) (bool, string) {
	switch {
	case in.Cancelled:
		return false, "request_cancelled"
	case in.Committed:
		return false, "response_started"
	case in.Settled:
		return false, "already_settled"
	case in.Strict:
		return false, "strict_affinity"
	case in.Skip:
		return false, "retry_disabled_by_affinity"
	case in.Unsafe:
		return false, "unsafe_replay"
	case !p.Retry:
		return false, "retry_disabled"
	case !verdict.Retry:
		return false, "not_retryable"
	case in.Attempts >= p.MaxAttempts:
		return false, "attempt_budget_exhausted"
	case in.Elapsed >= time.Duration(p.RetryWindow)*time.Second:
		return false, "retry_window_exhausted"
	default:
		return true, "retry_other_channel"
	}
}

package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"starhub/common"
)

// Four-level rate limiting (P4.4): global (IP) -> user -> token -> model.
// The IP and user limiters live in ratelimit.go; this file adds the token
// and model layers.
//
// Token limiting: each token gets its own bucket; the limit is configurable
// via ConfigureTokenRateLimit (0 = unlimited).
//
// Model limiting: each model gets its own bucket shared across all users;
// the limit is configurable via model_ratio option or a dedicated setting.
// A model-level limiter protects a single expensive model from being
// overwhelmed even when individual users are within their budget.

const (
	DefaultTokenLimitPerMinute = 60
	DefaultModelLimitPerMinute = 200
)

var (
	tokenLimiter = newFixedWindowLimiter(DefaultTokenLimitPerMinute)
	modelLimiter = newFixedWindowLimiter(DefaultModelLimitPerMinute)
)

// ConfigureTokenRateLimit sets the per-token request limit (0 = unlimited).
func ConfigureTokenRateLimit(perMinute int64) {
	if perMinute < 0 {
		perMinute = DefaultTokenLimitPerMinute
	}
	tokenLimiter.mu.Lock()
	tokenLimiter.limit = perMinute
	tokenLimiter.mu.Unlock()
}

// ConfigureModelRateLimit sets the per-model request limit (0 = unlimited).
func ConfigureModelRateLimit(perMinute int64) {
	if perMinute < 0 {
		perMinute = DefaultModelLimitPerMinute
	}
	modelLimiter.mu.Lock()
	modelLimiter.limit = perMinute
	modelLimiter.mu.Unlock()
}

// ResetTokenRateLimits clears the token limiter buckets (used by tests).
func ResetTokenRateLimits() {
	tokenLimiter.mu.Lock()
	tokenLimiter.buckets = map[string]*rateBucket{}
	tokenLimiter.mu.Unlock()
}

// ResetModelRateLimits clears the model limiter buckets (used by tests).
func ResetModelRateLimits() {
	modelLimiter.mu.Lock()
	modelLimiter.buckets = map[string]*rateBucket{}
	modelLimiter.mu.Unlock()
}

// TokenRateLimit limits requests per token (the third level of the four-tier
// rate limiting chain). It must run after TokenAuth (which writes
// CtxTokenID).
func TokenRateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenID, _ := r.Context().Value(common.CtxTokenID).(int64)
		if tokenID > 0 && tokenLimiter.limit > 0 &&
			!tokenLimiter.allow("token:"+strconv.FormatInt(tokenID, 10), time.Now()) {
			writeRateLimited(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ModelRateLimit limits requests per model (the fourth level). It must run
// after Distribute (which writes CtxRequestModel).
func ModelRateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		model, _ := r.Context().Value(common.CtxRequestModel).(string)
		if model != "" && modelLimiter.limit > 0 &&
			!modelLimiter.allow("model:"+model, time.Now()) {
			writeRateLimited(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// perModelLimits holds per-model override limits (model -> requests per minute).
// A model with no entry falls back to the global modelLimiter.
var (
	perModelMu      sync.RWMutex
	perModelLimits  = map[string]int64{}
	perModelBuckets = map[string]*fixedWindowLimiter{}
)

// SetModelRateLimit sets a per-model override limit (0 = unlimited).
func SetModelRateLimit(model string, perMinute int64) {
	perModelMu.Lock()
	defer perModelMu.Unlock()
	if perMinute <= 0 {
		delete(perModelLimits, model)
		delete(perModelBuckets, model)
		return
	}
	perModelLimits[model] = perMinute
	perModelBuckets[model] = newFixedWindowLimiter(perMinute)
}

// ModelRateLimitWithOverrides is ModelRateLimit with per-model override
// support: models with a configured override use their own limiter, others
// fall back to the global modelLimiter.
func ModelRateLimitWithOverrides(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		model, _ := r.Context().Value(common.CtxRequestModel).(string)
		if model == "" {
			next.ServeHTTP(w, r)
			return
		}
		now := time.Now()
		perModelMu.RLock()
		customLimiter, hasCustom := perModelBuckets[model]
		perModelMu.RUnlock()
		if hasCustom {
			if !customLimiter.allow("model:"+model, now) {
				writeRateLimited(w, r)
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		if modelLimiter.limit > 0 && !modelLimiter.allow("model:"+model, now) {
			writeRateLimited(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"starhub/common"
)

func TestTokenRateLimitPerToken(t *testing.T) {
	t.Cleanup(func() {
		ConfigureTokenRateLimit(DefaultTokenLimitPerMinute)
		ResetTokenRateLimits()
	})
	ConfigureTokenRateLimit(2)
	ResetTokenRateLimits()

	handler := TokenRateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	tokenReq := func(tokenID int64) *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		return req.WithContext(common.WithValues(req.Context(), common.CtxTokenID, tokenID))
	}
	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, tokenReq(1))
		if i < 2 && rec.Code != http.StatusOK {
			t.Fatalf("request %d for token 1 should pass, got %d", i, rec.Code)
		}
		if i == 2 && rec.Code != http.StatusTooManyRequests {
			t.Fatalf("request %d for token 1 should be limited, got %d", i, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, tokenReq(2))
	if rec.Code != http.StatusOK {
		t.Fatalf("another token must not be affected, got %d", rec.Code)
	}
}

func TestModelRateLimitPerModel(t *testing.T) {
	t.Cleanup(func() {
		ConfigureModelRateLimit(DefaultModelLimitPerMinute)
		ResetModelRateLimits()
	})
	ConfigureModelRateLimit(2)
	ResetModelRateLimits()

	handler := ModelRateLimitWithOverrides(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	modelReq := func(model string) *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		return req.WithContext(common.WithValues(req.Context(), common.CtxRequestModel, model))
	}
	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, modelReq("gpt-4o"))
		if i < 2 && rec.Code != http.StatusOK {
			t.Fatalf("request %d for model gpt-4o should pass, got %d", i, rec.Code)
		}
		if i == 2 && rec.Code != http.StatusTooManyRequests {
			t.Fatalf("request %d for model gpt-4o should be limited, got %d", i, rec.Code)
		}
	}
	// A different model is not affected.
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, modelReq("claude-3"))
	if rec.Code != http.StatusOK {
		t.Fatalf("a different model must not be limited, got %d", rec.Code)
	}
}

func TestModelRateLimitPerModelOverride(t *testing.T) {
	t.Cleanup(func() {
		ConfigureModelRateLimit(DefaultModelLimitPerMinute)
		ResetModelRateLimits()
		SetModelRateLimit("gpt-4o", 0)
	})
	ConfigureModelRateLimit(100) // high global limit
	ResetModelRateLimits()
	SetModelRateLimit("gpt-4o", 2) // tight per-model override

	handler := ModelRateLimitWithOverrides(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	modelReq := func(model string) *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		return req.WithContext(common.WithValues(req.Context(), common.CtxRequestModel, model))
	}
	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, modelReq("gpt-4o"))
		if i < 2 && rec.Code != http.StatusOK {
			t.Fatalf("request %d for model gpt-4o should pass, got %d", i, rec.Code)
		}
		if i == 2 && rec.Code != http.StatusTooManyRequests {
			t.Fatalf("request %d for model gpt-4o should be limited by override, got %d", i, rec.Code)
		}
	}
	// A model without override falls back to the global limit (100).
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, modelReq("gpt-3.5-turbo"))
	if rec.Code != http.StatusOK {
		t.Fatalf("model without override must not be limited, got %d", rec.Code)
	}
}
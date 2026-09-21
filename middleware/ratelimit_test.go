package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"starhub/common"
)

func restoreRateLimits(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		ConfigureRateLimits(DefaultIPLimitPerMinute, DefaultUserLimitPerMinute)
		ResetRateLimits()
	})
}

func TestIPRateLimitBlocksBurst(t *testing.T) {
	restoreRateLimits(t)
	ConfigureRateLimits(3, DefaultUserLimitPerMinute)
	ResetRateLimits()

	hits := 0
	handler := IPRateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/channel/", nil)
	req.RemoteAddr = "203.0.113.7:51234"
	for i := 0; i < 5; i++ {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if i < 3 && rec.Code != http.StatusOK {
			t.Fatalf("request %d should pass, got %d", i, rec.Code)
		}
		if i >= 3 && rec.Code != http.StatusTooManyRequests {
			t.Fatalf("request %d should be limited, got %d", i, rec.Code)
		}
	}
	if hits != 3 {
		t.Fatalf("downstream hits = %d, want 3", hits)
	}

	other := httptest.NewRequest(http.MethodGet, "/api/channel/", nil)
	other.RemoteAddr = "203.0.113.8:51234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, other)
	if rec.Code != http.StatusOK {
		t.Fatalf("a different IP must not be limited, got %d", rec.Code)
	}
}

func TestUserRateLimitPerUser(t *testing.T) {
	restoreRateLimits(t)
	ConfigureRateLimits(DefaultIPLimitPerMinute, 2)
	ResetRateLimits()

	handler := UserRateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	userReq := func(userID int64) *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		return req.WithContext(common.WithValues(req.Context(), common.CtxUserID, userID))
	}
	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, userReq(1))
		if i < 2 && rec.Code != http.StatusOK {
			t.Fatalf("request %d for user 1 should pass, got %d", i, rec.Code)
		}
		if i == 2 && rec.Code != http.StatusTooManyRequests {
			t.Fatalf("request %d for user 1 should be limited, got %d", i, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, userReq(2))
	if rec.Code != http.StatusOK {
		t.Fatalf("another user must not be affected, got %d", rec.Code)
	}
}

func TestRateLimitedResponseShape(t *testing.T) {
	restoreRateLimits(t)
	ConfigureRateLimits(1, DefaultUserLimitPerMinute)
	ResetRateLimits()

	handler := IPRateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	relayReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, relayReq)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, relayReq)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("relay request should be limited, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"rate_limit_error"`) || !strings.Contains(rec.Body.String(), `"error"`) {
		t.Fatalf("relay 429 should use the OpenAI error envelope: %s", rec.Body.String())
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("429 response should carry a Retry-After header")
	}

	ResetRateLimits()
	apiReq := httptest.NewRequest(http.MethodGet, "/api/channel/", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, apiReq)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, apiReq)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("management request should be limited, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"success":false`) {
		t.Fatalf("management 429 should use the API envelope: %s", rec.Body.String())
	}
}

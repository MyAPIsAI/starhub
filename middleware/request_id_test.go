package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestIDGenerated(t *testing.T) {
	called := false
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		id := CtxRequestIDValue(r.Context())
		if id == "" {
			t.Fatal("request ID should be in context")
		}
		if len(id) != 32 {
			t.Fatalf("request ID should be 32 hex chars, got %d", len(id))
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/channel", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if !called {
		t.Fatal("handler was not called")
	}
	if rr.Header().Get("X-StarHub-Request-Id") == "" {
		t.Fatal("response header should contain request ID")
	}
}

func TestRequestIDReusesIncoming(t *testing.T) {
	incomingID := "my-custom-request-id"
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := CtxRequestIDValue(r.Context())
		if id != incomingID {
			t.Fatalf("expected incoming ID %q, got %q", incomingID, id)
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/channel", nil)
	req.Header.Set("X-StarHub-Request-Id", incomingID)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Header().Get("X-StarHub-Request-Id") != incomingID {
		t.Fatalf("response header should reuse incoming ID, got %q", rr.Header().Get("X-StarHub-Request-Id"))
	}
}

func TestGenerateRequestIDUnique(t *testing.T) {
	id1 := GenerateRequestID()
	id2 := GenerateRequestID()
	if id1 == id2 {
		t.Fatal("two generated IDs should differ")
	}
}

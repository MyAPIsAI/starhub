package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestConsoleForwardsNativeProtocolPaths(t *testing.T) {
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("native API"))
	})
	handler := serveConsole(api, fstest.MapFS{"index.html": {Data: []byte("SPA")}})
	for _, path := range []string{"/api/user/self", "/v1/responses", "/v1beta/models/x:generateContent", "/v1beta/models/x:streamGenerateContent?alt=sse"} {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
			if rec.Code != http.StatusAccepted || rec.Body.String() != "native API" {
				t.Fatalf("embedded console intercepted %s %s: %d %s", method, path, rec.Code, rec.Body)
			}
		}
	}
	for _, path := range []string{"/v1beta-lookalike", "/console/playground", "/docs"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != 200 || rec.Body.String() != "SPA" {
			t.Fatalf("client route stopped serving SPA: %s %d %s", path, rec.Code, rec.Body)
		}
	}
}

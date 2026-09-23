package web

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
)

func TestBuiltConsole(t *testing.T) {
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	handler := Handler(api)
	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		return w
	}
	if w := get("/api/missing"); w.Code != http.StatusTeapot {
		t.Fatalf("API dispatch changed: %d", w.Code)
	}
	if !ConsoleEmbedded {
		if w := get("/login"); w.Code != http.StatusTeapot {
			t.Fatalf("API-only build must delegate to the API: %d", w.Code)
		}
		return
	}
	if w := get("/"); w.Code != http.StatusFound || w.Header().Get("Location") != "/console/overview" {
		t.Fatalf("console entry point unavailable: %d %v", w.Code, w.Header())
	}
	w := get("/login")
	if w.Code != http.StatusOK || !strings.Contains(w.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("embedded login page unavailable: %d %v", w.Code, w.Header())
	}
	assets := regexp.MustCompile(`(?:src|href)="(/assets/[^" ]+)"`).FindAllStringSubmatch(w.Body.String(), -1)
	if len(assets) == 0 {
		t.Fatal("embedded index must reference compiled frontend assets")
	}
	for _, asset := range assets {
		if w := get(asset[1]); w.Code != http.StatusOK || w.Body.Len() == 0 {
			t.Fatalf("embedded asset unavailable: %s (%d)", asset[1], w.Code)
		}
	}
}

func TestConsoleDispatch(t *testing.T) {
	files := fstest.MapFS{"index.html": {Data: []byte("<html>console</html>")}, "assets/app.js": {Data: []byte("export {}")}, "assets/app.js.map": {Data: []byte("secret")}}
	var root fs.FS = files
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-API", "true")
		http.Error(w, "api response", http.StatusTeapot)
	})
	handler := serveConsole(api, root)
	for _, tc := range []struct {
		method, path string
		code         int
		api, page    bool
	}{
		{"GET", "/", 302, false, false},
		{"GET", "/console/tokens", 200, false, true},
		{"HEAD", "/console/unknown", 200, false, true},
		{"GET", "/login", 200, false, true},
		{"GET", "/forbidden", 200, false, true},
		{"GET", "/assets/app.js", 200, false, false},
		{"GET", "/assets/missing.js", 404, false, false},
		{"GET", "/assets/app.js.map", 404, false, false},
		{"GET", "/console/missing.js", 404, false, false},
		{"GET", "/package.json", 404, false, false},
		{"GET", "/assets/../index.html", 404, false, false},
		{"POST", "/console/tokens", 405, false, false},
		{"GET", "/api", 418, true, false},
		{"GET", "/api/missing", 418, true, false},
		{"PATCH", "/api/user/self", 418, true, false},
		{"POST", "/v1/chat/completions", 418, true, false},
		{"GET", "/v1", 418, true, false},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
			if w.Code != tc.code || (w.Header().Get("X-API") == "true") != tc.api {
				t.Fatalf("unexpected response: %d %v %s", w.Code, w.Header(), w.Body.String())
			}
			if tc.page && w.Header().Get("Cache-Control") != "no-cache" {
				t.Fatal("index must revalidate")
			}
			if tc.path == "/assets/app.js" && !strings.Contains(w.Header().Get("Cache-Control"), "immutable") {
				t.Fatal("hashed assets must be immutable")
			}
			if tc.method == "HEAD" && w.Body.Len() != 0 {
				t.Fatal("HEAD must not send a body")
			}
		})
	}
}

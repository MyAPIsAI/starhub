package web

import (
	"bytes"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

// serveConsole routes API and relay traffic to api, and serves the
// embedded SPA for everything else. Asset files under /assets/ (and
// the favicon) are served verbatim with immutable caching; any other
// path without a file extension falls back to index.html so client
// routes (/console/*, /admin/*, /models/:id, ...) survive a refresh.
func serveConsole(api http.Handler, root fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if p == "/api" || strings.HasPrefix(p, "/api/") || p == "/v1" || strings.HasPrefix(p, "/v1/") || p == "/v1beta" || strings.HasPrefix(p, "/v1beta/") {
			api.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(p, "/")
		if name == "" || !fs.ValidPath(name) || strings.HasSuffix(name, ".map") || path.Clean(p) != p {
			// Root or invalid paths fall back to the SPA entry.
			name = "index.html"
		}
		isAsset := strings.HasPrefix(p, "/assets/") || p == "/favicon.svg"
		if !isAsset && path.Ext(name) == "" {
			// No file extension: treat as a client-side route.
			name = "index.html"
		}
		data, err := fs.ReadFile(root, name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if isAsset {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
	})
}

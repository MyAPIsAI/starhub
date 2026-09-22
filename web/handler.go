package web

import (
	"bytes"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

func serveConsole(api http.Handler, root fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if p == "/api" || strings.HasPrefix(p, "/api/") || p == "/v1" || strings.HasPrefix(p, "/v1/") {
			api.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if p == "/" {
			http.Redirect(w, r, "/console/overview", http.StatusFound)
			return
		}
		name := strings.TrimPrefix(p, "/")
		if !fs.ValidPath(name) || strings.HasSuffix(name, ".map") || path.Clean(p) != p {
			http.NotFound(w, r)
			return
		}
		page := p == "/login" || p == "/register" || p == "/forbidden" || p == "/console" || (strings.HasPrefix(p, "/console/") && path.Ext(p) == "")
		if page {
			name = "index.html"
		}
		if !page && !strings.HasPrefix(p, "/assets/") && p != "/favicon.svg" {
			http.NotFound(w, r)
			return
		}
		data, err := fs.ReadFile(root, name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if page {
			w.Header().Set("Cache-Control", "no-cache")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
	})
}

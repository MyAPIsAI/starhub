//go:build frontend

package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed dist dist/index.html
var assets embed.FS

// ConsoleEmbedded reports whether the binary includes the web console.
const ConsoleEmbedded = true

// Handler wraps next with the embedded console (web/dist). API and
// relay paths fall through to next; everything else is served from the
// embedded assets with an SPA fallback to index.html.
func Handler(next http.Handler) http.Handler {
	root, err := fs.Sub(assets, "dist")
	if err != nil {
		panic(err)
	}
	return serveConsole(next, root)
}
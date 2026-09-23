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

func Handler(api http.Handler) http.Handler {
	root, err := fs.Sub(assets, "dist")
	if err != nil {
		panic(err)
	}
	return serveConsole(api, root)
}

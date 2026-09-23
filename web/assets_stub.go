//go:build !frontend

package web

import "net/http"

// ConsoleEmbedded reports whether the binary includes the web console.
const ConsoleEmbedded = false

// Handler preserves the API-only build without requiring generated assets.
func Handler(api http.Handler) http.Handler { return api }

//go:build !frontend

// Package web exposes the embedded console wrapper. The default
// (API-only) build is a pass-through: Handler forwards to next and
// ConsoleEmbedded is false. Build with -tags frontend (see embed.go)
// to serve web/dist as the embedded console.
package web

import "net/http"

// ConsoleEmbedded reports whether the binary includes the web console.
// Always false in the API-only build.
const ConsoleEmbedded = false

// Handler preserves the API-only build without requiring generated
// assets; it is a pass-through to next.
func Handler(next http.Handler) http.Handler { return next }
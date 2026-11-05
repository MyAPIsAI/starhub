package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"starhub/common"
)

type ctxRequestIDKey struct{}

// GenerateRequestID produces a random 16-byte hex-encoded request ID.
func GenerateRequestID() string {
	raw := make([]byte, 16)
	_, _ = rand.Read(raw)
	return hex.EncodeToString(raw)
}

// CtxRequestIDValue returns the request ID from context, or "" if absent.
func CtxRequestIDValue(ctx context.Context) string {
	v, _ := ctx.Value(ctxRequestIDKey{}).(string)
	return v
}

// RequestID middleware generates a unique ID for every request, injects it
// into the context and sets the X-StarHub-Request-Id response header.
// If the incoming request already carries the header, it is reused.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-StarHub-Request-Id")
		if id == "" {
			id = GenerateRequestID()
		}
		w.Header().Set("X-StarHub-Request-Id", id)
		ctx := context.WithValue(r.Context(), ctxRequestIDKey{}, id)
		ctx = common.WithValues(ctx, common.CtxRequestID, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

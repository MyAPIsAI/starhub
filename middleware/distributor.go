package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"starhub/common"
)

const maxRequestBodySize = 32 << 20

func Distribute(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBodySize+1))
		if err != nil {
			common.RelayError(w, http.StatusBadRequest,
				"invalid_request_error", "read_body_failed",
				"failed to read request body: "+err.Error())
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		if len(body) > maxRequestBodySize {
			common.RelayError(w, http.StatusRequestEntityTooLarge,
				"invalid_request_error", "body_too_large",
				"request body exceeds the size limit")
			return
		}
		var lite struct {
			Model  string `json:"model"`
			Stream bool   `json:"stream"`
		}
		if err := json.Unmarshal(body, &lite); err != nil {
			common.RelayError(w, http.StatusBadRequest,
				"invalid_request_error", "invalid_json",
				"request body is not valid JSON: "+err.Error())
			return
		}
		if lite.Model == "" {
			common.RelayError(w, http.StatusBadRequest,
				"invalid_request_error", "model_required",
				"the \"model\" field is required")
			return
		}
		group, _ := r.Context().Value(common.CtxUsingGroup).(string)
		if group == "" {
			common.RelayError(w, http.StatusInternalServerError,
				"server_error", "internal_error",
				"using group is missing from request context")
			return
		}
		// Channel selection is left to the relay controller: it retries the
		// request on alternative channels, and every attempt needs a fresh
		// pick. The body stays cached in the context so each attempt can
		// replay it.
		ctx := common.WithValues(r.Context(),
			common.CtxRequestModel, lite.Model,
			common.CtxRequestBody, body,
			common.CtxRequestStream, lite.Stream)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"starhub/common"
	"starhub/model"
)

type requestLogKey struct{}

// RequestLogObservation is shared by the synchronous middleware and relay chain.
// It never retains credentials, prompts, response content, or provider errors.
type RequestLogObservation struct {
	Entry              model.Log
	Stage              string
	Outcome            int
	ClientDisconnected bool
}

func RequestObservation(ctx context.Context) *RequestLogObservation {
	value, _ := ctx.Value(requestLogKey{}).(*RequestLogObservation)
	return value
}

func SetRequestStage(ctx context.Context, stage string) {
	if value := RequestObservation(ctx); value != nil {
		value.Stage = stage
	}
}

func SetRequestFailure(ctx context.Context, stage, code, message string) {
	if value := RequestObservation(ctx); value != nil {
		value.Stage = stage
		value.Entry.FailureStage, value.Entry.ErrorCode, value.Entry.ErrorMessage = stage, code, message
	}
}

func observesRelay(r *http.Request) bool {
	return r.Method == http.MethodPost && (strings.HasPrefix(r.URL.Path, "/v1/") ||
		strings.HasPrefix(r.URL.Path, "/v1beta/models/") || strings.HasPrefix(r.URL.Path, "/api/playground/")) &&
		!strings.HasPrefix(r.URL.Path, "/v1/conversations") && !strings.HasSuffix(r.URL.Path, "/cancel")
}

// RequestLog wraps admission as well as dispatch, so rejected requests remain visible.
func RequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !observesRelay(r) {
			next.ServeHTTP(w, r)
			return
		}
		started := time.Now()
		// An untrusted caller-supplied ID must never identify another billing row.
		id := GenerateRequestID()
		w.Header().Set("X-StarHub-Request-Id", id)
		observation := &RequestLogObservation{Stage: "admission", Entry: model.Log{
			RequestID: id, RequestPath: boundedLogText(r.URL.Path, 256), RequestObserved: true,
			AdmittedAt: started.Unix(), Protocol: "OpenAI",
		}}
		if strings.HasPrefix(r.URL.Path, "/v1beta/") {
			observation.Entry.Protocol = "Gemini"
		}
		if r.URL.Path == "/v1/messages" {
			observation.Entry.Protocol = "Anthropic"
		}
		ctx := context.WithValue(r.Context(), ctxRequestIDKey{}, id)
		ctx = common.WithValues(ctx, common.CtxRequestID, id)
		ctx = context.WithValue(ctx, requestLogKey{}, observation)
		recorder := &requestLogWriter{ResponseWriter: w, observation: observation}
		completed := false
		defer func() {
			entry := &observation.Entry
			entry.HTTPStatus = recorder.status
			if entry.HTTPStatus == 0 && completed {
				entry.HTTPStatus = http.StatusOK
			}
			entry.Status = observation.Outcome
			if entry.Status == 0 {
				entry.Status = 1
				if entry.HTTPStatus >= 400 {
					entry.Status = 2
				}
			}
			if entry.Status == 1 && entry.HTTPStatus >= 400 {
				entry.Status = 2
			}
			if !completed {
				entry.Status = 2
				SetRequestFailure(ctx, observation.Stage, "request_interrupted", "Request processing did not complete.")
			}
			if observation.ClientDisconnected || errors.Is(r.Context().Err(), context.Canceled) {
				entry.Status = 3
			}
			if entry.Status != 1 {
				if entry.FailureStage == "" {
					entry.FailureStage = observation.Stage
				}
				if entry.ErrorCode == "" {
					entry.ErrorCode = safeResponseCode(recorder.body.Bytes())
				}
				if entry.Status == 3 {
					entry.ErrorCode = "request_cancelled"
				}
				if entry.ErrorCode == "" {
					entry.ErrorCode = "request_failed"
				}
				if entry.ErrorMessage == "" {
					entry.ErrorMessage = "Request did not complete. See the HTTP status and failure stage."
				}
			} else {
				entry.FailureStage, entry.ErrorCode, entry.ErrorMessage = "", "", ""
			}
			entry.AdmissionRejected = entry.Status != 1 && entry.UpstreamAttempts == 0
			entry.DurationMs = time.Since(started).Milliseconds()
			entry.CreatedAt = time.Now().Unix()
			if model.DB != nil {
				if err := model.FinalizeRequestLog(entry); err != nil {
					log.Printf("request log persistence failed: request=%s: %v", id, err)
				}
			}
		}()
		next.ServeHTTP(recorder, r.WithContext(ctx))
		completed = true
	})
}

func boundedLogText(value string, limit int) string {
	runes := []rune(value)
	if len(runes) > limit {
		runes = runes[:limit]
	}
	return string(runes)
}

var requestErrorCode = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)

func safeResponseCode(body []byte) string {
	var envelope struct {
		Error struct {
			Code string `json:"code"`
			Type string `json:"type"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		return ""
	}
	for _, code := range []string{envelope.Error.Code, envelope.Error.Type} {
		if requestErrorCode.MatchString(code) {
			return code
		}
	}
	return ""
}

type requestLogWriter struct {
	http.ResponseWriter
	observation *RequestLogObservation
	status      int
	body        bytes.Buffer
}

func (w *requestLogWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *requestLogWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	if status >= 100 && status < 200 {
		w.ResponseWriter.WriteHeader(status)
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *requestLogWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	if w.status >= 400 && w.body.Len() < 4096 {
		_, _ = w.body.Write(data[:min(len(data), 4096-w.body.Len())])
	}
	n, err := w.ResponseWriter.Write(data)
	if err != nil {
		w.observation.ClientDisconnected = true
	}
	return n, err
}
func (w *requestLogWriter) Flush() {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	if err := http.NewResponseController(w.ResponseWriter).Flush(); err != nil && !errors.Is(err, http.ErrNotSupported) {
		w.observation.ClientDisconnected = true
	}
}

// RecordRelayOutcome distinguishes a transport failure from an HTTP 200 stream.
func RecordRelayOutcome(ctx context.Context, err error) int {
	status := 1
	if err != nil {
		status = 2
	}
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		status = 3
	}
	if value := RequestObservation(ctx); value != nil {
		if value.ClientDisconnected {
			status = 3
		}
		value.Outcome = status
		if status != 1 {
			code := "upstream_response_incomplete"
			if status == 3 {
				code = "request_cancelled"
			}
			SetRequestFailure(ctx, "response", code, "The response was not completed.")
		}
	}
	return status
}

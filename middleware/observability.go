package middleware

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"strings"
	"time"

	"starhub/model"
	"starhub/observability"
)

type traceWriter struct {
	http.ResponseWriter
	status int
}

func (w *traceWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *traceWriter) WriteHeader(status int) {
	if w.status == 0 && (status >= 200 || status == 101) {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}
func (w *traceWriter) Write(raw []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	return w.ResponseWriter.Write(raw)
}
func (w *traceWriter) Flush() {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}
func (w *traceWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, rw, err := http.NewResponseController(w.ResponseWriter).Hijack()
	if err == nil {
		w.status = 101
	}
	return conn, rw, err
}

// ObserveGateway wraps admission too, so rejected calls retain a trace without a usage charge.
func ObserveGateway(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		generation := r.Method == "POST" && (strings.HasPrefix(r.URL.Path, "/v1/") || strings.HasPrefix(r.URL.Path, "/v1beta/"))
		if !generation && r.URL.Path != "/v1/realtime" {
			next.ServeHTTP(w, r)
			return
		}
		protocol := "OpenAI"
		if strings.HasPrefix(r.URL.Path, "/v1/messages") {
			protocol = "Anthropic"
		}
		if strings.HasPrefix(r.URL.Path, "/v1beta/") {
			protocol = "Gemini"
		}
		ctx, trace := observability.Start(r.Context(), model.LocalResources.ID, protocol)
		w.Header().Set("X-Request-ID", trace.Request.ID)
		writer := &traceWriter{ResponseWriter: w}
		defer func() {
			status := writer.status
			if status == 0 {
				status = 500
			}
			trace.Add(observability.Step{Stage: "gateway", StartedAt: trace.Request.StartedAt, EndedAt: time.Now().UnixMilli(), Status: traceOutcome(status)})
			request, steps := trace.Finish(status, r.Context().Err() != nil)
			saveCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := model.SaveTrace(saveCtx, request, steps); err != nil {
				observability.Log(request.ID, "trace_persistence", "failed")
			}
		}()
		next.ServeHTTP(writer, r.WithContext(ctx))
	})
}

func traceOutcome(status int) string {
	if status >= 200 && status < 400 || status == 101 {
		return "success"
	}
	return "failed"
}

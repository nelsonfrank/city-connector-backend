// Package obs provides the routing service's observability primitives:
// structured logging, request/correlation IDs and a minimal, dependency-free
// Prometheus text-format metrics registry (roadmap 0.8).
//
// The metrics registry is intentionally small. It can be swapped for the
// official prometheus/client_golang later without changing call sites much.
package obs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// RequestIDHeader is the correlation header propagated by Kong.
const RequestIDHeader = "X-Request-ID"

type ctxKey struct{}

// NewLogger returns a JSON slog logger at the given level.
func NewLogger(w io.Writer, level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level}))
}

// RequestID returns the request ID stored in ctx, or "" if none.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

// WithRequestID stores id in ctx.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

func newRequestID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// statusRecorder captures the response status for logging and metrics.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Middleware assigns/propagates a request ID, logs each request and records
// HTTP metrics. route should be a low-cardinality name (e.g. "journey").
func Middleware(log *slog.Logger, m *HTTPMetrics, route string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		id := r.Header.Get(RequestIDHeader)
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set(RequestIDHeader, id)
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r.WithContext(WithRequestID(r.Context(), id)))

		elapsed := time.Since(start)
		if m != nil {
			m.Observe(route, rec.status, elapsed)
		}
		log.LogAttrs(r.Context(), slog.LevelInfo, "http_request",
			slog.String("request_id", id),
			slog.String("route", route),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", rec.status),
			slog.Float64("duration_ms", float64(elapsed.Microseconds())/1000),
		)
	})
}


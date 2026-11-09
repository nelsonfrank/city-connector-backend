package api

import (
	"log/slog"
	"net/http"

	"github.com/city-connector-backend/routing/internal/obs"
)

// Readiness reports whether the service can serve journey queries, i.e. an
// in-memory timetable is loaded. Implemented by *timetable.Store.
type Readiness interface {
	Ready() (version string, ok bool)
}

// Server wires the HTTP endpoints of the routing service.
type Server struct {
	log     *slog.Logger
	ready   Readiness
	metrics *obs.Registry
	httpM   *obs.HTTPMetrics
}

// NewServer builds a Server. reg is used for /metrics and HTTP metrics.
func NewServer(log *slog.Logger, ready Readiness, reg *obs.Registry) *Server {
	return &Server{log: log, ready: ready, metrics: reg, httpM: obs.NewHTTPMetrics(reg)}
}

// Handler returns the root HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /journey", obs.Middleware(s.log, s.httpM, "journey", http.HandlerFunc(s.handleJourney)))
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /readyz", s.handleReadyz)
	mux.Handle("GET /metrics", s.metrics.Handler())
	return mux
}

// handleHealthz is the liveness probe: the process is up and serving HTTP.
func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleReadyz is the readiness probe: 200 only once a timetable is loaded,
// so Kubernetes doesn't route /journey traffic to a cold pod.
func (s *Server) handleReadyz(w http.ResponseWriter, _ *http.Request) {
	version, ok := s.ready.Ready()
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "loading"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready", "timetableVersion": version})
}

// handleJourney serves GET /journey (tech spec §3).
//
// Milestone M0: only the "timetable not loaded" contract is implemented.
// Request parsing, planning and response mapping arrive in milestone M4.
func (s *Server) handleJourney(w http.ResponseWriter, _ *http.Request) {
	if _, ok := s.ready.Ready(); !ok {
		writeError(w, http.StatusServiceUnavailable, CodeServiceUnavailable, "timetable not loaded yet")
		return
	}
	writeError(w, http.StatusNotImplemented, CodeNotImplemented, "journey planning is not implemented yet")
}


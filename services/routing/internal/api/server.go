// Package api wires HTTP endpoints for the routing service.
package api

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/city-connector-backend/routing/internal/config"
	"github.com/city-connector-backend/routing/internal/obs"
	"github.com/city-connector-backend/routing/internal/planner"
	"github.com/city-connector-backend/routing/internal/timetable"
)

// TimetableProvider returns the active timetable snapshot or nil.
type TimetableProvider interface {
	Current() *timetable.Timetable
	Ready() (version string, ok bool)
}

// JourneyPlanner computes itineraries from requests.
type JourneyPlanner interface {
	Plan(ctx context.Context, tt *timetable.Timetable, req planner.Request) (*planner.Response, error)
}

// Server wires the HTTP endpoints of the routing service.
type Server struct {
	cfg      config.Config
	log      *slog.Logger
	provider TimetableProvider
	planner  JourneyPlanner
	metrics  *obs.Registry
	httpM    *obs.HTTPMetrics
}

// NewServer builds a Server.
func NewServer(cfg config.Config, log *slog.Logger, provider TimetableProvider, pl JourneyPlanner, reg *obs.Registry) *Server {
	return &Server{
		cfg:      cfg,
		log:      log,
		provider: provider,
		planner:  pl,
		metrics:  reg,
		httpM:    obs.NewHTTPMetrics(reg),
	}
}

// Handler returns the root HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /journey", obs.Middleware(s.log, s.httpM, "journey", http.HandlerFunc(s.handleJourney)))
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /readyz", s.handleReadyz)
	mux.Handle("GET /metrics", s.metrics.Handler())

	// Swagger / OpenAPI documentation endpoints
	mux.HandleFunc("GET /docs", s.handleSwaggerUI)
	mux.HandleFunc("GET /docs/", s.handleSwaggerUI)
	mux.HandleFunc("GET /docs/openapi.json", s.handleOpenAPISpec)
	return mux
}

// handleHealthz is the liveness probe.
func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleReadyz is the readiness probe.
func (s *Server) handleReadyz(w http.ResponseWriter, _ *http.Request) {
	version, ok := s.provider.Ready()
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "loading"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready", "timetableVersion": version})
}

// handleJourney handles GET /journey conforming to tech spec §3.
func (s *Server) handleJourney(w http.ResponseWriter, r *http.Request) {
	tt := s.provider.Current()
	if tt == nil {
		writeError(w, http.StatusServiceUnavailable, CodeServiceUnavailable, "timetable not loaded yet")
		return
	}

	q := r.URL.Query()
	from := strings.TrimSpace(q.Get("from"))
	to := strings.TrimSpace(q.Get("to"))

	if from == "" || to == "" {
		writeError(w, http.StatusBadRequest, CodeInvalidLocation, "both 'from' and 'to' query parameters are required")
		return
	}

	// Parse optional time (RFC3339)
	var reqTime time.Time
	if tStr := strings.TrimSpace(q.Get("time")); tStr != "" {
		parsed, err := time.Parse(time.RFC3339, tStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, CodeInvalidParameter, "invalid 'time' format, expected ISO 8601 (RFC 3339)")
			return
		}
		reqTime = parsed
	}

	// Parse maxTransfers
	maxTransfers := s.cfg.DefaultMaxTransfers
	if mtStr := strings.TrimSpace(q.Get("maxTransfers")); mtStr != "" {
		val, err := strconv.Atoi(mtStr)
		if err != nil || val < 0 || val > s.cfg.MaxMaxTransfers {
			writeError(w, http.StatusBadRequest, CodeInvalidParameter, "invalid 'maxTransfers'")
			return
		}
		maxTransfers = val
	}

	// Parse maxWalkMeters
	maxWalkMeters := s.cfg.DefaultMaxWalkMeters
	if mwStr := strings.TrimSpace(q.Get("maxWalkMeters")); mwStr != "" {
		val, err := strconv.Atoi(mwStr)
		if err != nil || val < 0 || val > s.cfg.MaxMaxWalkMeters {
			writeError(w, http.StatusBadRequest, CodeInvalidParameter, "invalid 'maxWalkMeters'")
			return
		}
		maxWalkMeters = val
	}

	arriveBy := q.Get("arriveBy") == "true"

	req := planner.Request{
		From:          from,
		To:            to,
		Time:          reqTime,
		ArriveBy:      arriveBy,
		MaxTransfers:  maxTransfers,
		MaxWalkMeters: maxWalkMeters,
	}

	resp, err := s.planner.Plan(r.Context(), tt, req)
	if err != nil {
		errStr := err.Error()
		if strings.HasPrefix(errStr, "invalid_location:") {
			writeError(w, http.StatusBadRequest, CodeInvalidLocation, errStr)
			return
		}
		if strings.HasPrefix(errStr, "time_out_of_range:") {
			writeError(w, http.StatusUnprocessableEntity, CodeTimeOutOfRange, errStr)
			return
		}
		s.log.Error("journey plan error", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "an error occurred processing journey")
		return
	}

	writeJSON(w, http.StatusOK, resp)
}


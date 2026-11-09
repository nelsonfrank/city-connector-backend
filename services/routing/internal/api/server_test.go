package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/city-connector-backend/routing/internal/config"
	"github.com/city-connector-backend/routing/internal/obs"
	"github.com/city-connector-backend/routing/internal/planner"
	"github.com/city-connector-backend/routing/internal/raptor"
	"github.com/city-connector-backend/routing/internal/timetable"
)

func newTestServer(t *testing.T) (*timetable.Store, http.Handler) {
	t.Helper()
	store := timetable.NewStore()
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	cfg, _ := config.Load()
	pl := planner.NewPlanner(cfg, raptor.NewEngine(), nil)
	return store, NewServer(cfg, log, store, pl, obs.NewRegistry()).Handler()
}

func do(h http.Handler, method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestHealthz(t *testing.T) {
	_, h := newTestServer(t)
	if rec := do(h, http.MethodGet, "/healthz"); rec.Code != http.StatusOK {
		t.Fatalf("healthz = %d", rec.Code)
	}
}

func TestReadyzTracksTimetable(t *testing.T) {
	store, h := newTestServer(t)
	if rec := do(h, http.MethodGet, "/readyz"); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("readyz before load = %d, want 503", rec.Code)
	}
	store.Swap(&timetable.Timetable{Version: "mbta-2026-11-01"})
	rec := do(h, http.MethodGet, "/readyz")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "mbta-2026-11-01") {
		t.Fatalf("readyz after load = %d %s", rec.Code, rec.Body)
	}
}

func TestJourneyServiceUnavailableBeforeLoad(t *testing.T) {
	_, h := newTestServer(t)
	rec := do(h, http.MethodGet, "/journey?from=a&to=b")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("journey = %d, want 503", rec.Code)
	}
	var body ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Error != CodeServiceUnavailable {
		t.Fatalf("body = %s (err %v)", rec.Body, err)
	}
	if rec.Header().Get(obs.RequestIDHeader) == "" {
		t.Error("missing request id header")
	}
}

func TestJourneyPropagatesRequestID(t *testing.T) {
	_, h := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/journey", nil)
	req.Header.Set(obs.RequestIDHeader, "abc123")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get(obs.RequestIDHeader); got != "abc123" {
		t.Fatalf("request id = %q, want abc123", got)
	}
}

func TestJourneyMethodNotAllowed(t *testing.T) {
	_, h := newTestServer(t)
	if rec := do(h, http.MethodPost, "/journey"); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /journey = %d, want 405", rec.Code)
	}
}

func TestMetricsExposed(t *testing.T) {
	_, h := newTestServer(t)
	do(h, http.MethodGet, "/journey")
	rec := do(h, http.MethodGet, "/metrics")
	body := rec.Body.String()
	for _, want := range []string{
		`http_requests_total{route="journey",code="503"} 1`,
		`http_request_duration_seconds_bucket{route="journey",le="+Inf"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics missing %q\n%s", want, body)
		}
	}
}


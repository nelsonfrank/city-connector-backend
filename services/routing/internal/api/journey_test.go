package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/city-connector-backend/routing/internal/gtfs"
	"github.com/city-connector-backend/routing/internal/planner"
	"github.com/city-connector-backend/routing/internal/timetable"
)

func createSampleAPITimetable() *timetable.Timetable {
	day, _ := gtfs.ParseDate("20261109")
	feed := &timetable.RawFeed{
		Version: "v1.0",
		Agencies: []timetable.RawAgency{
			{ID: "MBTA", Name: "MBTA", Timezone: "UTC"},
		},
		Stops: []timetable.RawStop{
			{ID: "MBTA:1", Name: "Park St", Lat: 42.3563, Lon: -71.0624},
			{ID: "MBTA:2", Name: "South Station", Lat: 42.3522, Lon: -71.0552},
		},
		Routes: []timetable.RawRoute{
			{ID: "MBTA:Red", AgencyID: "MBTA", ShortName: "Red", LongName: "Red Line", Type: 1},
		},
		Calendars: []timetable.RawCalendar{
			{
				ServiceID: "DAILY",
				Monday:    true, Tuesday: true, Wednesday: true, Thursday: true, Friday: true, Saturday: true, Sunday: true,
				StartDate: day.AddDays(-5),
				EndDate:   day.AddDays(5),
			},
		},
		Trips: []timetable.RawTrip{
			{ID: "MBTA:T1", RouteID: "MBTA:Red", ServiceID: "DAILY", Direction: 0},
		},
		StopTimes: []timetable.RawStopTime{
			{TripID: "MBTA:T1", StopID: "MBTA:1", StopSequence: 1, ArrTime: 8 * 3600, DepTime: 8 * 3600},
			{TripID: "MBTA:T1", StopID: "MBTA:2", StopSequence: 2, ArrTime: 8*3600 + 300, DepTime: 8*3600 + 300},
		},
	}
	b := timetable.NewBuilder(timetable.DefaultBuilderConfig())
	tt, _ := b.Build(feed)
	return tt
}

func TestJourneySuccess(t *testing.T) {
	store, h := newTestServer(t)
	tt := createSampleAPITimetable()
	store.Swap(tt)

	reqTime := time.Date(2026, 11, 9, 7, 55, 0, 0, time.UTC).Format(time.RFC3339)
	rec := do(h, http.MethodGet, "/journey?from=MBTA:1&to=MBTA:2&time="+reqTime)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body)
	}

	var resp planner.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if len(resp.Itineraries) != 1 {
		t.Fatalf("expected 1 itinerary, got %d", len(resp.Itineraries))
	}
	itin := resp.Itineraries[0]
	if itin.Rank != "fastest" {
		t.Errorf("rank = %q, want fastest", itin.Rank)
	}
	if len(itin.Legs) != 1 {
		t.Errorf("legs = %d, want 1", len(itin.Legs))
	}
	if itin.Legs[0].RouteName != "Red" {
		t.Errorf("routeName = %q, want Red", itin.Legs[0].RouteName)
	}
}

func TestJourneyValidationErrors(t *testing.T) {
	store, h := newTestServer(t)
	tt := createSampleAPITimetable()
	store.Swap(tt)

	// Missing from/to
	rec := do(h, http.MethodGet, "/journey?from=MBTA:1")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("missing to = %d, want 400", rec.Code)
	}

	// Invalid time format
	rec = do(h, http.MethodGet, "/journey?from=MBTA:1&to=MBTA:2&time=not-a-date")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid time = %d, want 400", rec.Code)
	}

	// Unknown stop ID
	rec = do(h, http.MethodGet, "/journey?from=UNKNOWN:999&to=MBTA:2")
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), CodeInvalidLocation) {
		t.Errorf("unknown stop = %d, body = %s", rec.Code, rec.Body)
	}

	// Date out of calendar range
	pastTime := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	rec = do(h, http.MethodGet, "/journey?from=MBTA:1&to=MBTA:2&time="+pastTime)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), CodeTimeOutOfRange) {
		t.Errorf("out of range date = %d, body = %s", rec.Code, rec.Body)
	}
}

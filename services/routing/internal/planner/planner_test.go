package planner

import (
	"context"
	"testing"
	"time"

	"github.com/city-connector-backend/routing/internal/config"
	"github.com/city-connector-backend/routing/internal/gtfs"
	"github.com/city-connector-backend/routing/internal/raptor"
	"github.com/city-connector-backend/routing/internal/timetable"
)

func createTestTimetable() (*timetable.Timetable, gtfs.Date) {
	day, _ := gtfs.ParseDate("20261109")
	feed := &timetable.RawFeed{
		Version: "test",
		Agencies: []timetable.RawAgency{
			{ID: "MBTA", Name: "MBTA", Timezone: "UTC"},
		},
		Stops: []timetable.RawStop{
			{ID: "MBTA:1", Name: "Park St", Lat: 42.3563, Lon: -71.0624},
			{ID: "MBTA:2", Name: "Downtown Crossing", Lat: 42.3555, Lon: -71.0605},
			{ID: "MBTA:3", Name: "South Station", Lat: 42.3522, Lon: -71.0552},
		},
		Routes: []timetable.RawRoute{
			{ID: "MBTA:Red", AgencyID: "MBTA", ShortName: "Red Line", Type: 1},
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
			{ID: "MBTA:Trip1", RouteID: "MBTA:Red", ServiceID: "DAILY", Direction: 0},
		},
		StopTimes: []timetable.RawStopTime{
			{TripID: "MBTA:Trip1", StopID: "MBTA:1", StopSequence: 1, ArrTime: 8 * 3600, DepTime: 8 * 3600},
			{TripID: "MBTA:Trip1", StopID: "MBTA:2", StopSequence: 2, ArrTime: 8*3600 + 120, DepTime: 8*3600 + 120},
			{TripID: "MBTA:Trip1", StopID: "MBTA:3", StopSequence: 3, ArrTime: 8*3600 + 300, DepTime: 8*3600 + 300},
		},
	}
	b := timetable.NewBuilder(timetable.DefaultBuilderConfig())
	tt, _ := b.Build(feed)
	return tt, day
}

func TestPlannerDirectStops(t *testing.T) {
	tt, _ := createTestTimetable()
	cfg, _ := config.Load()
	p := NewPlanner(cfg, raptor.NewEngine(), nil)

	reqTime := time.Date(2026, 11, 9, 7, 50, 0, 0, time.UTC)
	resp, err := p.Plan(context.Background(), tt, Request{
		From:          "MBTA:1",
		To:            "MBTA:3",
		Time:          reqTime,
		MaxTransfers:  2,
		MaxWalkMeters: 500,
	})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	if len(resp.Itineraries) == 0 {
		t.Fatal("expected at least one itinerary")
	}
	itin := resp.Itineraries[0]
	if itin.Rank != "fastest" {
		t.Errorf("rank = %q, want fastest", itin.Rank)
	}
	if itin.Transfers != 0 {
		t.Errorf("transfers = %d, want 0", itin.Transfers)
	}
	if len(itin.Legs) != 1 {
		t.Fatalf("legs = %d, want 1 direct transit leg", len(itin.Legs))
	}
	if itin.Legs[0].RouteName != "Red Line" {
		t.Errorf("routeName = %q, want Red Line", itin.Legs[0].RouteName)
	}
}

func TestPlannerCoordinatesNearby(t *testing.T) {
	tt, _ := createTestTimetable()
	cfg, _ := config.Load()
	p := NewPlanner(cfg, raptor.NewEngine(), nil)

	reqTime := time.Date(2026, 11, 9, 7, 50, 0, 0, time.UTC)
	// Coords ~100m away from Park St
	resp, err := p.Plan(context.Background(), tt, Request{
		From:          "42.3570,-71.0620",
		To:            "MBTA:3",
		Time:          reqTime,
		MaxTransfers:  2,
		MaxWalkMeters: 500,
	})
	if err != nil {
		t.Fatalf("plan with coordinates: %v", err)
	}
	if len(resp.Itineraries) == 0 {
		t.Fatal("expected itinerary with access walk leg")
	}
	itin := resp.Itineraries[0]
	if itin.Legs[0].Mode != "walk" {
		t.Errorf("first leg mode = %q, want walk", itin.Legs[0].Mode)
	}
}

func TestPlannerSameStopEdgeCase(t *testing.T) {
	tt, _ := createTestTimetable()
	cfg, _ := config.Load()
	p := NewPlanner(cfg, raptor.NewEngine(), nil)

	reqTime := time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)
	resp, err := p.Plan(context.Background(), tt, Request{
		From:          "MBTA:1",
		To:            "MBTA:1",
		Time:          reqTime,
		MaxTransfers:  2,
		MaxWalkMeters: 500,
	})
	if err != nil {
		t.Fatalf("same stop query: %v", err)
	}
	if len(resp.Itineraries) != 0 {
		t.Errorf("expected empty itineraries, got %d", len(resp.Itineraries))
	}
	if resp.Message == "" {
		t.Error("expected explanation message")
	}
}

func TestPlannerTimeOutOfRange(t *testing.T) {
	tt, _ := createTestTimetable()
	cfg, _ := config.Load()
	p := NewPlanner(cfg, raptor.NewEngine(), nil)

	pastTime := time.Date(2020, 1, 1, 8, 0, 0, 0, time.UTC)
	_, err := p.Plan(context.Background(), tt, Request{
		From:          "MBTA:1",
		To:            "MBTA:3",
		Time:          pastTime,
		MaxTransfers:  2,
		MaxWalkMeters: 500,
	})
	if err == nil {
		t.Fatal("expected time_out_of_range error for 2020 date")
	}
}

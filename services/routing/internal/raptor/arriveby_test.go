package raptor_test

import (
	"context"
	"testing"
	"time"

	"github.com/city-connector-backend/routing/internal/config"
	"github.com/city-connector-backend/routing/internal/planner"
	"github.com/city-connector-backend/routing/internal/raptor"
)

func TestGoldenArriveByRoute(t *testing.T) {
	tt := loadGoldenTimetable(t)
	cfg, _ := config.Load()
	p := planner.NewPlanner(cfg, raptor.NewEngine(), nil)

	// Goal: Arrive at South Terminal before 08:35 on 2026-11-09.
	// In sample-feed:
	// TRIP_BLUE_1 arrives at 08:14
	// TRIP_BLUE_2 arrives at 08:34
	// Backward search should pick TRIP_BLUE_2 (latest departure from North at 08:20)
	targetArrival := time.Date(2026, 11, 9, 8, 35, 0, 0, time.UTC)

	resp, err := p.Plan(context.Background(), tt, planner.Request{
		From:          "DEMO:STOP_NORTH",
		To:            "DEMO:STOP_SOUTH",
		Time:          targetArrival,
		ArriveBy:      true,
		MaxTransfers:  2,
		MaxWalkMeters: 500,
	})
	if err != nil {
		t.Fatalf("arriveBy plan error: %v", err)
	}

	if len(resp.Itineraries) == 0 {
		t.Fatal("expected at least one itinerary for arriveBy query")
	}

	best := resp.Itineraries[0]
	// Trip departs North at 08:20 and arrives South at 08:34
	expectedDeparture := "2026-11-09T08:20:00Z"
	expectedArrival := "2026-11-09T08:34:00Z"

	if best.DepartAt != expectedDeparture {
		t.Errorf("departAt = %q, want %q", best.DepartAt, expectedDeparture)
	}
	if best.ArriveAt != expectedArrival {
		t.Errorf("arriveAt = %q, want %q", best.ArriveAt, expectedArrival)
	}
}


package raptor_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/city-connector-backend/routing/internal/config"
	"github.com/city-connector-backend/routing/internal/planner"
	"github.com/city-connector-backend/routing/internal/raptor"
	"github.com/city-connector-backend/routing/internal/timetable"
)

func loadGoldenTimetable(t *testing.T) *timetable.Timetable {
	t.Helper()
	feedPath := filepath.Join("..", "..", "testdata", "sample-feed")
	src := &timetable.GTFSZipSource{
		Path:          feedPath,
		DefaultAgency: "DEMO",
	}
	raw, err := src.Load(context.Background())
	if err != nil {
		t.Fatalf("load sample feed: %v", err)
	}

	builder := timetable.NewBuilder(timetable.DefaultBuilderConfig())
	tt, err := builder.Build(raw)
	if err != nil {
		t.Fatalf("build timetable: %v", err)
	}
	return tt
}

func TestGoldenDirectRoute(t *testing.T) {
	tt := loadGoldenTimetable(t)
	cfg, _ := config.Load()
	p := planner.NewPlanner(cfg, raptor.NewEngine(), nil)

	// North Terminal to South Terminal at 07:55 on a Monday (2026-11-09)
	reqTime := time.Date(2026, 11, 9, 7, 55, 0, 0, time.UTC)
	resp, err := p.Plan(context.Background(), tt, planner.Request{
		From:          "DEMO:STOP_NORTH",
		To:            "DEMO:STOP_SOUTH",
		Time:          reqTime,
		MaxTransfers:  2,
		MaxWalkMeters: 500,
	})
	if err != nil {
		t.Fatalf("plan error: %v", err)
	}

	if len(resp.Itineraries) == 0 {
		t.Fatal("expected at least one itinerary for North -> South")
	}

	best := resp.Itineraries[0]
	if best.Rank != "fastest" {
		t.Errorf("rank = %q, want fastest", best.Rank)
	}
	if best.Transfers != 0 {
		t.Errorf("transfers = %d, want 0 (direct Blue line)", best.Transfers)
	}

	// Trip arrives at 08:14
	expectedArrival := "2026-11-09T08:14:00Z"
	if best.ArriveAt != expectedArrival {
		t.Errorf("arriveAt = %q, want %q", best.ArriveAt, expectedArrival)
	}
}

func TestGoldenTransferRoute(t *testing.T) {
	tt := loadGoldenTimetable(t)
	cfg, _ := config.Load()
	p := planner.NewPlanner(cfg, raptor.NewEngine(), nil)

	// West Plaza to Airport at 08:00
	// Route: Red line from West Plaza (08:05) -> Central Station (08:12),
	// Transfer to Green line at Central Station (08:15) -> Airport (08:30)
	reqTime := time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)
	resp, err := p.Plan(context.Background(), tt, planner.Request{
		From:          "DEMO:STOP_WEST",
		To:            "DEMO:STOP_AIRPORT",
		Time:          reqTime,
		MaxTransfers:  2,
		MaxWalkMeters: 500,
	})
	if err != nil {
		t.Fatalf("plan error: %v", err)
	}

	if len(resp.Itineraries) == 0 {
		t.Fatal("expected multi-leg itinerary from West to Airport")
	}

	best := resp.Itineraries[0]
	if best.Transfers != 1 {
		t.Errorf("transfers = %d, want 1", best.Transfers)
	}

	expectedArrival := "2026-11-09T08:30:00Z"
	if best.ArriveAt != expectedArrival {
		t.Errorf("arriveAt = %q, want %q", best.ArriveAt, expectedArrival)
	}
}

func BenchmarkGoldenJourneySearch(b *testing.B) {
	feedPath := filepath.Join("..", "..", "testdata", "sample-feed")
	src := &timetable.GTFSZipSource{Path: feedPath, DefaultAgency: "DEMO"}
	raw, _ := src.Load(context.Background())
	builder := timetable.NewBuilder(timetable.DefaultBuilderConfig())
	tt, _ := builder.Build(raw)

	cfg, _ := config.Load()
	p := planner.NewPlanner(cfg, raptor.NewEngine(), nil)
	reqTime := time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = p.Plan(ctx, tt, planner.Request{
			From:          "DEMO:STOP_WEST",
			To:            "DEMO:STOP_AIRPORT",
			Time:          reqTime,
			MaxTransfers:  2,
			MaxWalkMeters: 500,
		})
	}
}

package raptor

import (
	"context"
	"testing"

	"github.com/city-connector-backend/routing/internal/gtfs"
	"github.com/city-connector-backend/routing/internal/timetable"
)

func buildTestNetwork() (*timetable.Timetable, gtfs.Date) {
	// Simple network:
	// Route 1 (bus): S1 -> S2 -> S3
	// Route 2 (bus): S3 -> S4 -> S5
	// Route 3 (express): S1 -> S5 (departs later, faster)
	// Walking transfer between S2 and S4 (300 meters)

	day, _ := gtfs.ParseDate("20261109")

	feed := &timetable.RawFeed{
		Version: "test-feed",
		Agencies: []timetable.RawAgency{
			{ID: "A", Name: "Test Agency", Timezone: "UTC"},
		},
		Stops: []timetable.RawStop{
			{ID: "S1", Name: "Origin Stop", Lat: 0.000, Lon: 0.000},
			{ID: "S2", Name: "Transfer Stop A", Lat: 0.010, Lon: 0.000},
			{ID: "S3", Name: "Junction Stop", Lat: 0.020, Lon: 0.000},
			{ID: "S4", Name: "Transfer Stop B", Lat: 0.012, Lon: 0.000},
			{ID: "S5", Name: "Destination Stop", Lat: 0.030, Lon: 0.000},
		},
		Routes: []timetable.RawRoute{
			{ID: "R1", AgencyID: "A", ShortName: "1", LongName: "Route 1", Type: 3},
			{ID: "R2", AgencyID: "A", ShortName: "2", LongName: "Route 2", Type: 3},
		},
		Calendars: []timetable.RawCalendar{
			{
				ServiceID: "DAILY",
				Monday:    true, Tuesday: true, Wednesday: true, Thursday: true, Friday: true, Saturday: true, Sunday: true,
				StartDate: day.AddDays(-10),
				EndDate:   day.AddDays(10),
			},
		},
		Trips: []timetable.RawTrip{
			{ID: "T1", RouteID: "R1", ServiceID: "DAILY", Direction: 0},
			{ID: "T2", RouteID: "R2", ServiceID: "DAILY", Direction: 0},
		},
		StopTimes: []timetable.RawStopTime{
			// T1: S1 (08:00) -> S2 (08:10) -> S3 (08:20)
			{TripID: "T1", StopID: "S1", StopSequence: 1, ArrTime: 8 * 3600, DepTime: 8 * 3600},
			{TripID: "T1", StopID: "S2", StopSequence: 2, ArrTime: 8*3600 + 10*60, DepTime: 8*3600 + 10*60},
			{TripID: "T1", StopID: "S3", StopSequence: 3, ArrTime: 8*3600 + 20*60, DepTime: 8*3600 + 20*60},

			// T2: S3 (08:25) -> S4 (08:35) -> S5 (08:45)
			{TripID: "T2", StopID: "S3", StopSequence: 1, ArrTime: 8*3600 + 25*60, DepTime: 8*3600 + 25*60},
			{TripID: "T2", StopID: "S4", StopSequence: 2, ArrTime: 8*3600 + 35*60, DepTime: 8*3600 + 35*60},
			{TripID: "T2", StopID: "S5", StopSequence: 3, ArrTime: 8*3600 + 45*60, DepTime: 8*3600 + 45*60},
		},
	}

	b := timetable.NewBuilder(timetable.DefaultBuilderConfig())
	tt, err := b.Build(feed)
	if err != nil {
		panic(err)
	}
	return tt, day
}

func TestRaptorSingleTransitLeg(t *testing.T) {
	tt, day := buildTestNetwork()
	engine := NewEngine()

	// S1 to S3
	s1Idx := tt.StopByID["S1"]
	s3Idx := tt.StopByID["S3"]

	q := Query{
		AccessStops:  []StopArrival{{StopIdx: s1Idx, WalkSeconds: 0, WalkMeters: 0}},
		EgressStops:  []StopArrival{{StopIdx: s3Idx, WalkSeconds: 0, WalkMeters: 0}},
		Departure:    7*3600 + 50*60, // 07:50
		ServiceDay:   day,
		MaxTransfers: 1,
	}

	journeys := engine.Route(context.Background(), tt, q)
	if len(journeys) == 0 {
		t.Fatal("expected journey from S1 to S3")
	}

	jn := journeys[0]
	if jn.Transfers != 0 {
		t.Errorf("transfers = %d, want 0", jn.Transfers)
	}
	if jn.ArriveAt != 8*3600+20*60 { // 08:20
		t.Errorf("arrival = %d (%s), want 08:20:00", jn.ArriveAt, gtfs.FormatTime(jn.ArriveAt))
	}
}

func TestRaptorTransferJourney(t *testing.T) {
	tt, day := buildTestNetwork()
	engine := NewEngine()

	// S1 to S5 requires transfer at S3 (T1 -> T2)
	s1Idx := tt.StopByID["S1"]
	s5Idx := tt.StopByID["S5"]

	q := Query{
		AccessStops:  []StopArrival{{StopIdx: s1Idx, WalkSeconds: 60, WalkMeters: 50}},
		EgressStops:  []StopArrival{{StopIdx: s5Idx, WalkSeconds: 120, WalkMeters: 100}},
		Departure:    7 * 3600, // 07:00
		ServiceDay:   day,
		MaxTransfers: 2,
	}

	journeys := engine.Route(context.Background(), tt, q)
	if len(journeys) == 0 {
		t.Fatal("expected multi-leg journey from S1 to S5")
	}

	jn := journeys[0]
	if jn.Transfers != 1 {
		t.Errorf("transfers = %d, want 1", jn.Transfers)
	}
	// Arrives at S5 at 08:45 + 120s egress walk = 08:47
	expectedArrival := gtfs.Seconds(8*3600 + 45*60 + 120)
	if jn.ArriveAt != expectedArrival {
		t.Errorf("arrival = %d (%s), want %s", jn.ArriveAt, gtfs.FormatTime(jn.ArriveAt), gtfs.FormatTime(expectedArrival))
	}

	if len(jn.Legs) < 3 {
		t.Fatalf("expected at least access, 2 transit legs, and egress; got %d legs", len(jn.Legs))
	}
}

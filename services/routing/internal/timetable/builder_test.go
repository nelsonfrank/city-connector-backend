package timetable

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/city-connector-backend/routing/internal/gtfs"
)

func createSampleGTFSDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	write("agency.txt", "agency_id,agency_name,agency_url,agency_timezone\nA1,Agency One,http://example.com,UTC\n")
	write("stops.txt", `stop_id,stop_name,stop_lat,stop_lon
S1,Stop 1,42.3601,-71.0589
S2,Stop 2,42.3610,-71.0580
S3,Stop 3,42.3650,-71.0550
`)
	write("routes.txt", "route_id,agency_id,route_short_name,route_long_name,route_type\nR1,A1,1,Route 1,3\n")
	write("calendar.txt", "service_id,monday,tuesday,wednesday,thursday,friday,saturday,sunday,start_date,end_date\nSERV1,1,1,1,1,1,1,1,20261101,20261130\n")
	write("trips.txt", "route_id,service_id,trip_id,trip_headsign,direction_id\nR1,SERV1,T1,Outbound,0\nR1,SERV1,T2,Outbound,0\n")
	write("stop_times.txt", `trip_id,arrival_time,departure_time,stop_id,stop_sequence
T1,08:00:00,08:00:00,S1,1
T1,08:10:00,08:10:00,S2,2
T1,08:20:00,08:20:00,S3,3
T2,08:30:00,08:30:00,S1,1
T2,08:40:00,08:40:00,S2,2
T2,08:50:00,08:50:00,S3,3
`)
	return dir
}

func TestBuilderFromDir(t *testing.T) {
	dir := createSampleGTFSDir(t)
	src := &GTFSZipSource{Path: dir, DefaultAgency: "TEST"}

	raw, err := src.Load(context.Background())
	if err != nil {
		t.Fatalf("load raw feed: %v", err)
	}

	if len(raw.Stops) != 3 {
		t.Fatalf("expected 3 stops, got %d", len(raw.Stops))
	}

	builder := NewBuilder(DefaultBuilderConfig())
	tt, err := builder.Build(raw)
	if err != nil {
		t.Fatalf("build timetable: %v", err)
	}

	if len(tt.Stops) != 3 {
		t.Errorf("expected 3 stops in timetable, got %d", len(tt.Stops))
	}
	if len(tt.Patterns) != 1 {
		t.Errorf("expected 1 pattern for identical stop sequence, got %d", len(tt.Patterns))
	}
	p := tt.Patterns[0]
	if p.NumTrips != 2 {
		t.Errorf("expected 2 trips in pattern, got %d", p.NumTrips)
	}
	if p.NumStops != 3 {
		t.Errorf("expected 3 stops in pattern, got %d", p.NumStops)
	}

	// Verify calendar bitset
	d, _ := gtfs.ParseDate("20261109")
	if !tt.IsServiceActive(0, d) {
		t.Errorf("expected service to be active on 20261109")
	}

	// Verify spatial grid index
	nearby := tt.Spatial.FindNearby(42.3601, -71.0589, 200.0)
	if len(nearby) < 1 {
		t.Errorf("expected nearby stops around S1, got %d", len(nearby))
	}
}


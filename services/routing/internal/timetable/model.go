package timetable

import "time"

// Timetable is an immutable, RAPTOR-ready snapshot of the active GTFS data.
//
// It is built once per feed version by the Builder and then only ever read
// (concurrently) by the RAPTOR engine. It must never be mutated after it is
// published to a Store.
//
// NOTE: only the version metadata exists so far (milestone M0). The flat
// stop/pattern/trip arrays described in the routing engine plan §3.2 are
// added in milestone M1.
type Timetable struct {
	// Version identifies the feed version(s) this timetable was built from.
	Version string
	// BuiltAt is when the builder finished constructing this snapshot.
	BuiltAt time.Time
}


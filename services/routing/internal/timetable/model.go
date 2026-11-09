package timetable

import (
	"time"

	"github.com/city-connector-backend/routing/internal/gtfs"
)

// Dense typed indices for RAPTOR array storage.
type StopIdx uint32
type PatternIdx uint32
type TripIdx uint32
type ServiceIdx uint32
type RouteIdx uint32
type AgencyIdx uint32

const InvalidStopIdx StopIdx = ^StopIdx(0)
const InvalidPatternIdx PatternIdx = ^PatternIdx(0)
const InvalidTripIdx TripIdx = ^TripIdx(0)

// DateRange tracks inclusive service dates covered by the timetable.
type DateRange struct {
	Start gtfs.Date
	End   gtfs.Date
}

// Stop represents a public transit stop or platform.
type Stop struct {
	ID        string
	Name      string
	Lat       float64
	Lon       float64
	AgencyIdx AgencyIdx
}

// Route represents a GTFS route metadata.
type Route struct {
	ID        string
	AgencyIdx AgencyIdx
	ShortName string
	LongName  string
	Type      int // GTFS route_type (0: tram, 1: subway, 2: rail, 3: bus, etc.)
}

// Agency holds agency metadata.
type Agency struct {
	ID       string
	Name     string
	Timezone string
}

// StopTime represents scheduled arrival and departure in seconds from service day start.
type StopTime struct {
	Arr gtfs.Seconds
	Dep gtfs.Seconds
}

// Trip represents a single scheduled vehicle journey along a pattern.
type Trip struct {
	ID         string
	PatternIdx PatternIdx
	ServiceIdx ServiceIdx
	Headsign   string
}

// Pattern groups trips that serve the exact same sequence of stops in FIFO order.
type Pattern struct {
	RouteIdx RouteIdx
	StopsOff uint32
	NumStops uint16
	TripsOff uint32
	NumTrips uint32
	TimesOff uint32 // Offset into StopTimes: StopTimes[TimesOff + tripOffset*NumStops + stopSeq]
}

// PatternStopRef notes that a stop is visited by a pattern at a given stop index.
type PatternStopRef struct {
	PatternIdx PatternIdx
	StopPos    uint16
}

// Transfer represents a precomputed footpath to a nearby stop.
type Transfer struct {
	To     StopIdx
	Secs   uint16
	Meters uint16
}

// Timetable is an immutable, RAPTOR-ready in-memory snapshot of GTFS static data.
type Timetable struct {
	Version string
	BuiltAt time.Time
	TZ      *time.Location
	Horizon DateRange

	Stops    []Stop
	StopByID map[string]StopIdx

	Patterns     []Pattern
	PatternStops []StopIdx
	StopTimes    []StopTime
	Trips        []Trip

	// Stop -> Patterns visiting this stop
	StopPatterns    []PatternStopRef
	StopPatternsOff []uint32

	// Transfers (footpaths) from stop
	Transfers    []Transfer
	TransfersOff []uint32

	// Service Calendars (active date bitsets)
	Services    []gtfs.Bitset
	ServiceByID map[string]ServiceIdx

	Routes   []Route
	Agencies []Agency

	Spatial *SpatialGrid
}

// NumDays returns the number of calendar days spanned by the horizon.
func (h DateRange) NumDays() int {
	if h.End < h.Start {
		return 0
	}
	return int(h.End - h.Start + 1)
}

// Contains reports whether date is within the range.
func (h DateRange) Contains(d gtfs.Date) bool {
	return d >= h.Start && d <= h.End
}

// IsServiceActive reports whether a service_id is active on a given date.
func (tt *Timetable) IsServiceActive(s ServiceIdx, d gtfs.Date) bool {
	if int(s) >= len(tt.Services) || !tt.Horizon.Contains(d) {
		return false
	}
	bitIdx := int(d - tt.Horizon.Start)
	return tt.Services[s].Has(bitIdx)
}

// GetStopTime returns the StopTime for a given trip index (pattern-local offset) and stop sequence in the pattern.
func (tt *Timetable) GetStopTime(p *Pattern, tripOffset uint32, stopSeq uint16) StopTime {
	idx := p.TimesOff + tripOffset*uint32(p.NumStops) + uint32(stopSeq)
	return tt.StopTimes[idx]
}

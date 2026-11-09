package raptor

import (
	"github.com/city-connector-backend/routing/internal/gtfs"
	"github.com/city-connector-backend/routing/internal/timetable"
)

// LegMode identifies the type of travel leg in a reconstructed itinerary.
type LegMode string

const (
	LegModeWalk    LegMode = "walk"
	LegModeTransit LegMode = "transit"
)

// EndpointPoint specifies a stop or coordinate endpoint.
type EndpointPoint struct {
	StopID   string   `json:"stopId,omitempty"`
	StopName string   `json:"name,omitempty"`
	Lat      float64  `json:"lat"`
	Lon      float64  `json:"lng"`
}

// JourneyLeg represents an individual travel segment (walk, bus, subway, etc.).
type JourneyLeg struct {
	Mode            LegMode          `json:"mode"`
	DurationSeconds int              `json:"durationSeconds"`
	From            EndpointPoint    `json:"from"`
	To              EndpointPoint    `json:"to"`
	DepartAt        gtfs.Seconds     `json:"departAt"`
	ArriveAt        gtfs.Seconds     `json:"arriveAt"`
	AgencyID        string           `json:"agencyId,omitempty"`
	RouteID         string           `json:"routeId,omitempty"`
	RouteShortName  string           `json:"routeShortName,omitempty"`
	RouteLongName   string           `json:"routeLongName,omitempty"`
	TripID          string           `json:"tripId,omitempty"`
	Headsign        string           `json:"headsign,omitempty"`
	StopSequence    []string         `json:"stopSequence,omitempty"`
	WalkMeters      int              `json:"walkMeters,omitempty"`
}

// Journey is an end-to-end trip option discovered by the RAPTOR engine.
type Journey struct {
	DepartAt        gtfs.Seconds
	ArriveAt        gtfs.Seconds
	DurationSeconds int
	Transfers       int
	WalkMeters      int
	Legs            []JourneyLeg
}

// StopArrival is an access/egress candidate with walk time and distance.
type StopArrival struct {
	StopIdx     timetable.StopIdx
	WalkSeconds int32
	WalkMeters  int
}

// Query defines the inputs for the RAPTOR routing computation.
type Query struct {
	AccessStops  []StopArrival
	EgressStops  []StopArrival
	Departure    gtfs.Seconds
	ServiceDay   gtfs.Date
	MaxTransfers int
}


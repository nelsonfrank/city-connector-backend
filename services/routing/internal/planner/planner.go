package planner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/city-connector-backend/routing/internal/config"
	"github.com/city-connector-backend/routing/internal/eta"
	"github.com/city-connector-backend/routing/internal/gtfs"
	"github.com/city-connector-backend/routing/internal/raptor"
	"github.com/city-connector-backend/routing/internal/timetable"
)

// Request defines the parsed parameters for journey planning.
type Request struct {
	From          string // "lat,lng" or namespaced stop ID
	To            string // "lat,lng" or namespaced stop ID
	Time          time.Time
	ArriveBy      bool
	MaxTransfers  int
	MaxWalkMeters int
}

// Location represents a coordinate or named stop in the response.
type Location struct {
	StopID string  `json:"stopId,omitempty"`
	Name   string  `json:"name,omitempty"`
	Lat    float64 `json:"lat,omitempty"`
	Lng    float64 `json:"lng,omitempty"`
}

// Leg represents a single segment of an itinerary.
type Leg struct {
	Mode            string     `json:"mode"`
	DurationSeconds int        `json:"durationSeconds"`
	From            Location   `json:"from"`
	To              Location   `json:"to"`
	DepartAt        *time.Time `json:"departAt,omitempty"`
	ArriveAt        *time.Time `json:"arriveAt,omitempty"`
	AgencyID        string     `json:"agencyId,omitempty"`
	RouteID         string     `json:"routeId,omitempty"`
	RouteName       string     `json:"routeName,omitempty"`
	TripID          string     `json:"tripId,omitempty"`
	DelaySeconds    int        `json:"delaySeconds,omitempty"`
	StopSequence    []string   `json:"stopSequence,omitempty"`
	WalkMeters      int        `json:"walkMeters,omitempty"`
}

// Itinerary is a ranked option presented in the journey search response.
type Itinerary struct {
	ID              string `json:"id"`
	Rank            string `json:"rank"` // "fastest", "fewest_transfers", "least_walking"
	DepartAt        string `json:"departAt"`
	ArriveAt        string `json:"arriveAt"`
	DurationSeconds int    `json:"durationSeconds"`
	Transfers       int    `json:"transfers"`
	WalkMeters      int    `json:"walkMeters"`
	Realtime        bool   `json:"realtime"`
	Legs            []Leg  `json:"legs"`
}

// QuerySummary mirrors the input query in the response.
type QuerySummary struct {
	From     Location `json:"from"`
	To       Location `json:"to"`
	DepartAt string   `json:"departAt"`
}

// Response is the canonical top-level payload conforming to tech spec §3.
type Response struct {
	Query       QuerySummary `json:"query"`
	Itineraries []Itinerary  `json:"itineraries"`
	Message     string       `json:"message,omitempty"`
}

// Planner coordinates endpoint resolution, RAPTOR search, ranking, and response formatting.
type Planner struct {
	cfg      config.Config
	engine   *raptor.Engine
	adjuster eta.Adjuster
}

func NewPlanner(cfg config.Config, engine *raptor.Engine, adjuster eta.Adjuster) *Planner {
	if adjuster == nil {
		adjuster = eta.NewNoopAdjuster()
	}
	return &Planner{
		cfg:      cfg,
		engine:   engine,
		adjuster: adjuster,
	}
}

// Plan executes the full journey planning lifecycle.
func (p *Planner) Plan(ctx context.Context, tt *timetable.Timetable, req Request) (*Response, error) {
	resp := &Response{
		Itineraries: []Itinerary{},
	}

	// 1. Resolve Time & Service Day
	reqTime := req.Time
	if reqTime.IsZero() {
		reqTime = time.Now().In(tt.TZ)
	} else {
		reqTime = reqTime.In(tt.TZ)
	}
	serviceDay := gtfs.DateOf(reqTime)

	// Range validation (Tech spec §3: 422 time_out_of_range)
	if !tt.Horizon.Contains(serviceDay) {
		return nil, fmt.Errorf("time_out_of_range: requested date %s is outside schedule horizon [%s, %s]",
			serviceDay, tt.Horizon.Start, tt.Horizon.End)
	}

	startInstant := gtfs.ServiceDayStart(serviceDay, tt.TZ)
	depSeconds := gtfs.Seconds(reqTime.Sub(startInstant).Seconds())

	// 2. Resolve From and To Endpoints
	fromLoc, accessStops, err := p.resolveEndpoint(tt, req.From, req.MaxWalkMeters)
	if err != nil {
		return nil, fmt.Errorf("invalid_location: from: %w", err)
	}
	toLoc, egressStops, err := p.resolveEndpoint(tt, req.To, req.MaxWalkMeters)
	if err != nil {
		return nil, fmt.Errorf("invalid_location: to: %w", err)
	}

	resp.Query = QuerySummary{
		From:     fromLoc,
		To:       toLoc,
		DepartAt: reqTime.UTC().Format(time.RFC3339),
	}

	// Edge case: Origin and destination resolve to the same stop (Tech spec §6)
	if fromLoc.StopID != "" && toLoc.StopID != "" && fromLoc.StopID == toLoc.StopID {
		resp.Message = "Origin and destination are the same location"
		return resp, nil
	}
	if len(accessStops) == 0 || len(egressStops) == 0 {
		resp.Message = "No reachable stops found within walking distance"
		return resp, nil
	}

	// 3. Execute RAPTOR Engine
	q := raptor.Query{
		AccessStops:  accessStops,
		EgressStops:  egressStops,
		Departure:    depSeconds,
		ServiceDay:   serviceDay,
		MaxTransfers: req.MaxTransfers,
	}

	rawJourneys := p.engine.Route(ctx, tt, q)
	if len(rawJourneys) == 0 {
		return resp, nil
	}

	// 4. Adjust with Realtime Delays
	adjusted := p.adjuster.Adjust(ctx, rawJourneys)

	// 5. Rank & Format Itineraries
	itins := p.rankAndFormat(tt, serviceDay, adjusted, fromLoc, toLoc)
	resp.Itineraries = itins
	return resp, nil
}

func (p *Planner) resolveEndpoint(tt *timetable.Timetable, input string, maxWalkMeters int) (Location, []raptor.StopArrival, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return Location{}, nil, fmt.Errorf("missing endpoint")
	}

	// Check if coordinate "lat,lng"
	if parts := strings.Split(input, ","); len(parts) == 2 {
		lat, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		lng, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err1 == nil && err2 == nil {
			if lat < -90 || lat > 90 || lng < -180 || lng > 180 {
				return Location{}, nil, fmt.Errorf("coordinates out of range")
			}
			loc := Location{Lat: lat, Lng: lng}

			// Spatial grid query
			nearby := tt.Spatial.FindNearby(lat, lng, float64(maxWalkMeters))
			walkSpeed := p.cfg.WalkSpeedMPS
			if walkSpeed <= 0 {
				walkSpeed = 1.33
			}

			arrivals := make([]raptor.StopArrival, 0, len(nearby))
			for _, cand := range nearby {
				walkSecs := int32(math.Ceil((cand.Meters * 1.3) / walkSpeed))
				arrivals = append(arrivals, raptor.StopArrival{
					StopIdx:     cand.StopIdx,
					WalkSeconds: walkSecs,
					WalkMeters:  int(cand.Meters),
				})
			}
			return loc, arrivals, nil
		}
	}

	// Otherwise treat as Stop ID
	sIdx, found := tt.StopByID[input]
	if !found {
		return Location{}, nil, fmt.Errorf("unknown stop_id %q", input)
	}
	s := &tt.Stops[sIdx]
	loc := Location{
		StopID: s.ID,
		Name:   s.Name,
		Lat:    s.Lat,
		Lng:    s.Lon,
	}
	return loc, []raptor.StopArrival{
		{StopIdx: sIdx, WalkSeconds: 0, WalkMeters: 0},
	}, nil
}

func (p *Planner) rankAndFormat(tt *timetable.Timetable, day gtfs.Date, journeys []raptor.Journey, fromLoc, toLoc Location) []Itinerary {
	if len(journeys) == 0 {
		return nil
	}

	// 1. Identify "fastest", "fewest_transfers", "least_walking"
	fastestIdx := 0
	fewestTransfersIdx := 0
	leastWalkingIdx := 0

	for i := 1; i < len(journeys); i++ {
		if journeys[i].ArriveAt < journeys[fastestIdx].ArriveAt {
			fastestIdx = i
		}
		if journeys[i].Transfers < journeys[fewestTransfersIdx].Transfers {
			fewestTransfersIdx = i
		}
		if journeys[i].WalkMeters < journeys[leastWalkingIdx].WalkMeters {
			leastWalkingIdx = i
		}
	}

	assigned := make(map[int]string)
	assigned[fastestIdx] = "fastest"

	if _, ok := assigned[fewestTransfersIdx]; !ok {
		assigned[fewestTransfersIdx] = "fewest_transfers"
	}
	if _, ok := assigned[leastWalkingIdx]; !ok {
		assigned[leastWalkingIdx] = "least_walking"
	}

	result := make([]Itinerary, 0, len(assigned))
	for idx, jn := range journeys {
		rank, hasRank := assigned[idx]
		if !hasRank {
			rank = "alternative"
		}

		depTime := gtfs.Instant(day, jn.DepartAt, tt.TZ).UTC()
		arrTime := gtfs.Instant(day, jn.ArriveAt, tt.TZ).UTC()

		legs := make([]Leg, 0, len(jn.Legs))
		for legIdx, l := range jn.Legs {
			legDep := gtfs.Instant(day, l.DepartAt, tt.TZ).UTC()
			legArr := gtfs.Instant(day, l.ArriveAt, tt.TZ).UTC()

			fromPt := Location{
				StopID: l.From.StopID,
				Name:   l.From.StopName,
				Lat:    l.From.Lat,
				Lng:    l.From.Lon,
			}
			toPt := Location{
				StopID: l.To.StopID,
				Name:   l.To.StopName,
				Lat:    l.To.Lat,
				Lng:    l.To.Lon,
			}

			// Access origin coordinate
			if legIdx == 0 && l.Mode == raptor.LegModeWalk && fromPt.StopID == "" {
				fromPt = fromLoc
			}
			// Egress destination coordinate
			if legIdx == len(jn.Legs)-1 && l.Mode == raptor.LegModeWalk && toPt.StopID == "" {
				toPt = toLoc
			}

			routeName := l.RouteShortName
			if routeName == "" {
				routeName = l.RouteLongName
			}

			legs = append(legs, Leg{
				Mode:            string(l.Mode),
				DurationSeconds: l.DurationSeconds,
				From:            fromPt,
				To:              toPt,
				DepartAt:        &legDep,
				ArriveAt:        &legArr,
				AgencyID:        l.AgencyID,
				RouteID:         l.RouteID,
				RouteName:       routeName,
				TripID:          l.TripID,
				StopSequence:    l.StopSequence,
				WalkMeters:      l.WalkMeters,
			})
		}

		itinID := hashItinerary(legs)
		result = append(result, Itinerary{
			ID:              itinID,
			Rank:            rank,
			DepartAt:        depTime.Format(time.RFC3339),
			ArriveAt:        arrTime.Format(time.RFC3339),
			DurationSeconds: jn.DurationSeconds,
			Transfers:       jn.Transfers,
			WalkMeters:      jn.WalkMeters,
			Realtime:        false,
			Legs:            legs,
		})
	}

	return result
}

func hashItinerary(legs []Leg) string {
	h := sha256.New()
	for _, l := range legs {
		h.Write([]byte(l.Mode))
		h.Write([]byte(l.RouteID))
		h.Write([]byte(l.TripID))
		h.Write([]byte(l.From.StopID))
		h.Write([]byte(l.To.StopID))
	}
	return "itin_" + hex.EncodeToString(h.Sum(nil))[:8]
}

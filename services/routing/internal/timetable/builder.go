package timetable

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/city-connector-backend/routing/internal/gtfs"
)

// BuilderConfig parameterizes timetable construction.
type BuilderConfig struct {
	WalkSpeedMPS      float64
	MaxTransferMeters int
	MinTransferSecs   int
}

// DefaultBuilderConfig provides standard defaults.
func DefaultBuilderConfig() BuilderConfig {
	return BuilderConfig{
		WalkSpeedMPS:      1.33,
		MaxTransferMeters: 500,
		MinTransferSecs:   60,
	}
}

// Builder compiles a RawFeed into an immutable, indexed, RAPTOR-ready Timetable.
type Builder struct {
	cfg BuilderConfig
}

func NewBuilder(cfg BuilderConfig) *Builder {
	return &Builder{cfg: cfg}
}

// Build validates, unpacks frequencies, indexes, groups patterns, and generates transfers.
func (b *Builder) Build(raw *RawFeed) (*Timetable, error) {
	if len(raw.Stops) == 0 {
		return nil, fmt.Errorf("builder: feed contains no stops")
	}
	if len(raw.Trips) == 0 {
		return nil, fmt.Errorf("builder: feed contains no trips")
	}
	if len(raw.StopTimes) == 0 {
		return nil, fmt.Errorf("builder: feed contains no stop_times")
	}

	tt := &Timetable{
		Version:  raw.Version,
		BuiltAt:  time.Now().UTC(),
		StopByID: make(map[string]StopIdx, len(raw.Stops)),
	}

	// 1. Resolve Timezone
	tzName := "UTC"
	if len(raw.Agencies) > 0 && raw.Agencies[0].Timezone != "" {
		tzName = raw.Agencies[0].Timezone
	}
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		return nil, fmt.Errorf("invalid timezone %q: %w", tzName, err)
	}
	tt.TZ = loc

	// 2. Index Agencies
	agencyMap := make(map[string]AgencyIdx, len(raw.Agencies))
	for _, a := range raw.Agencies {
		idx := AgencyIdx(len(tt.Agencies))
		tt.Agencies = append(tt.Agencies, Agency{
			ID:       a.ID,
			Name:     a.Name,
			Timezone: a.Timezone,
		})
		agencyMap[a.ID] = idx
	}

	// 3. Index Stops
	for _, s := range raw.Stops {
		if _, exists := tt.StopByID[s.ID]; exists {
			continue // ignore duplicate stop IDs
		}
		idx := StopIdx(len(tt.Stops))
		tt.Stops = append(tt.Stops, Stop{
			ID:   s.ID,
			Name: s.Name,
			Lat:  s.Lat,
			Lon:  s.Lon,
		})
		tt.StopByID[s.ID] = idx
	}

	// 4. Index Routes
	routeMap := make(map[string]RouteIdx, len(raw.Routes))
	for _, r := range raw.Routes {
		idx := RouteIdx(len(tt.Routes))
		tt.Routes = append(tt.Routes, Route{
			ID:        r.ID,
			AgencyIdx: agencyMap[r.AgencyID],
			ShortName: r.ShortName,
			LongName:  r.LongName,
			Type:      r.Type,
		})
		routeMap[r.ID] = idx
	}

	// 5. Expand Frequencies if present
	trips := raw.Trips
	stopTimes := raw.StopTimes
	if len(raw.Frequencies) > 0 {
		trips, stopTimes = b.expandFrequencies(raw.Trips, raw.StopTimes, raw.Frequencies)
	}

	// 6. Build Calendar / Horizon
	if err := b.buildCalendars(tt, raw.Calendars, raw.CalendarDates); err != nil {
		return nil, fmt.Errorf("calendar: %w", err)
	}

	// 7. Group StopTimes by Trip
	tripStopTimes := make(map[string][]RawStopTime)
	for _, st := range stopTimes {
		tripStopTimes[st.TripID] = append(tripStopTimes[st.TripID], st)
	}

	// Sort stop_times of each trip by stop_sequence
	for tripID, sts := range tripStopTimes {
		sort.Slice(sts, func(i, j int) bool {
			return sts[i].StopSequence < sts[j].StopSequence
		})
		// Interpolate missing times if any
		interpolateTimes(sts)
		tripStopTimes[tripID] = sts
	}

	// 8. Create Patterns & Trips
	if err := b.buildPatterns(tt, trips, tripStopTimes, routeMap); err != nil {
		return nil, fmt.Errorf("patterns: %w", err)
	}

	// 9. Invert Stop -> Pattern Index
	b.buildStopPatterns(tt)

	// 10. Generate Precomputed Transfers (Transfers + Footpaths)
	b.buildTransfers(tt, raw.Transfers)

	// 11. Spatial Grid Index
	tt.Spatial = NewSpatialGrid(tt.Stops, 250.0)

	return tt, nil
}

// buildCalendars parses calendar and calendar_dates into active day bitsets
func (b *Builder) buildCalendars(tt *Timetable, calendars []RawCalendar, calendarDates []RawCalendarDate) error {
	var minDate, maxDate gtfs.Date = math.MaxInt32, 0

	for _, c := range calendars {
		if c.StartDate < minDate {
			minDate = c.StartDate
		}
		if c.EndDate > maxDate {
			maxDate = c.EndDate
		}
	}
	for _, cd := range calendarDates {
		if cd.Date < minDate {
			minDate = cd.Date
		}
		if cd.Date > maxDate {
			maxDate = cd.Date
		}
	}

	if minDate > maxDate {
		// No service calendar found; construct a 1-day fallback
		today := gtfs.DateOf(time.Now().In(tt.TZ))
		minDate = today
		maxDate = today
	}

	tt.Horizon = DateRange{Start: minDate, End: maxDate}
	numDays := tt.Horizon.NumDays()

	serviceIDs := make(map[string]ServiceIdx)
	for _, c := range calendars {
		if _, ok := serviceIDs[c.ServiceID]; !ok {
			serviceIDs[c.ServiceID] = ServiceIdx(len(tt.Services))
			tt.Services = append(tt.Services, gtfs.NewBitset(numDays))
		}
	}
	for _, cd := range calendarDates {
		if _, ok := serviceIDs[cd.ServiceID]; !ok {
			serviceIDs[cd.ServiceID] = ServiceIdx(len(tt.Services))
			tt.Services = append(tt.Services, gtfs.NewBitset(numDays))
		}
	}
	tt.ServiceByID = serviceIDs

	// Apply base calendar
	for _, c := range calendars {
		sIdx := serviceIDs[c.ServiceID]
		bs := tt.Services[sIdx]

		cur := c.StartDate
		for cur <= c.EndDate {
			if tt.Horizon.Contains(cur) {
				dayOfWeek := cur.Weekday()
				active := false
				switch dayOfWeek {
				case time.Monday:
					active = c.Monday
				case time.Tuesday:
					active = c.Tuesday
				case time.Wednesday:
					active = c.Wednesday
				case time.Thursday:
					active = c.Thursday
				case time.Friday:
					active = c.Friday
				case time.Saturday:
					active = c.Saturday
				case time.Sunday:
					active = c.Sunday
				}
				if active {
					bit := int(cur - tt.Horizon.Start)
					bs.Set(bit)
				}
			}
			cur = cur.AddDays(1)
		}
	}

	// Apply exceptions
	for _, cd := range calendarDates {
		sIdx, ok := serviceIDs[cd.ServiceID]
		if !ok || !tt.Horizon.Contains(cd.Date) {
			continue
		}
		bit := int(cd.Date - tt.Horizon.Start)
		if cd.ExceptionType == 1 {
			tt.Services[sIdx].Set(bit)
		} else if cd.ExceptionType == 2 {
			tt.Services[sIdx].Clear(bit)
		}
	}

	return nil
}

type patternKey string

func makePatternKey(routeID string, dir int, stops []StopIdx) patternKey {
	b := make([]byte, 0, len(routeID)+8+len(stops)*4)
	b = append(b, routeID...)
	b = fmt.Appendf(b, ":%d:", dir)
	for _, s := range stops {
		b = fmt.Appendf(b, "%d,", s)
	}
	return patternKey(b)
}

type tripWithTimes struct {
	rawTrip   RawTrip
	stopTimes []RawStopTime
}

func (b *Builder) buildPatterns(tt *Timetable, trips []RawTrip, tripStopTimes map[string][]RawStopTime, routeMap map[string]RouteIdx) error {
	// Group trips by stop sequence pattern
	patternGroups := make(map[patternKey][]tripWithTimes)

	for _, tr := range trips {
		sts, ok := tripStopTimes[tr.ID]
		if !ok || len(sts) < 2 {
			continue // ignore trips with fewer than 2 stops
		}
		// Map stops to StopIdx
		stopIndices := make([]StopIdx, len(sts))
		valid := true
		for i, st := range sts {
			sIdx, found := tt.StopByID[st.StopID]
			if !found {
				valid = false
				break
			}
			stopIndices[i] = sIdx
		}
		if !valid {
			continue
		}

		key := makePatternKey(tr.RouteID, tr.Direction, stopIndices)
		patternGroups[key] = append(patternGroups[key], tripWithTimes{rawTrip: tr, stopTimes: sts})
	}

	// Sort and build each pattern
	for _, group := range patternGroups {
		if len(group) == 0 {
			continue
		}
		// Sort trips by departure time at the first stop (FIFO)
		sort.Slice(group, func(i, j int) bool {
			return group[i].stopTimes[0].DepTime < group[j].stopTimes[0].DepTime
		})

		// Detect and split overtaking trips into separate FIFO patterns
		subGroups := splitOvertaking(group)

		for _, sub := range subGroups {
			if len(sub) == 0 {
				continue
			}
			numStops := uint16(len(sub[0].stopTimes))
			patternIdx := PatternIdx(len(tt.Patterns))

			routeIdx := routeMap[sub[0].rawTrip.RouteID]
			stopsOff := uint32(len(tt.PatternStops))
			for _, st := range sub[0].stopTimes {
				tt.PatternStops = append(tt.PatternStops, tt.StopByID[st.StopID])
			}

			tripsOff := uint32(len(tt.Trips))
			timesOff := uint32(len(tt.StopTimes))

			for _, tr := range sub {
				serviceIdx := tt.ServiceByID[tr.rawTrip.ServiceID]
				tt.Trips = append(tt.Trips, Trip{
					ID:         tr.rawTrip.ID,
					PatternIdx: patternIdx,
					ServiceIdx: serviceIdx,
					Headsign:   tr.rawTrip.Headsign,
				})

				for _, st := range tr.stopTimes {
					tt.StopTimes = append(tt.StopTimes, StopTime{
						Arr: st.ArrTime,
						Dep: st.DepTime,
					})
				}
			}

			tt.Patterns = append(tt.Patterns, Pattern{
				RouteIdx: routeIdx,
				StopsOff: stopsOff,
				NumStops: numStops,
				TripsOff: tripsOff,
				NumTrips: uint32(len(sub)),
				TimesOff: timesOff,
			})
		}
	}

	return nil
}

// splitOvertaking ensures strict FIFO order (trip A earlier than B at stop 0 must not arrive later than B at any stop).
func splitOvertaking(group []tripWithTimes) [][]tripWithTimes {
	var subGroups [][]tripWithTimes

Outer:
	for _, tr := range group {
		for i := range subGroups {
			last := subGroups[i][len(subGroups[i])-1]
			if isFIFO(last, tr) {
				subGroups[i] = append(subGroups[i], tr)
				continue Outer
			}
		}
		subGroups = append(subGroups, []tripWithTimes{tr})
	}
	return subGroups
}

func isFIFO(earlier, later tripWithTimes) bool {
	for i := range earlier.stopTimes {
		if earlier.stopTimes[i].DepTime > later.stopTimes[i].DepTime ||
			earlier.stopTimes[i].ArrTime > later.stopTimes[i].ArrTime {
			return false
		}
	}
	return true
}

func (b *Builder) buildStopPatterns(tt *Timetable) {
	numStops := len(tt.Stops)
	stopRefs := make([][]PatternStopRef, numStops)

	for pIdx, p := range tt.Patterns {
		for pos := uint16(0); pos < p.NumStops; pos++ {
			sIdx := tt.PatternStops[p.StopsOff+uint32(pos)]
			stopRefs[sIdx] = append(stopRefs[sIdx], PatternStopRef{
				PatternIdx: PatternIdx(pIdx),
				StopPos:    pos,
			})
		}
	}

	tt.StopPatternsOff = make([]uint32, numStops+1)
	var totalRefs uint32
	for sIdx, refs := range stopRefs {
		tt.StopPatternsOff[sIdx] = totalRefs
		totalRefs += uint32(len(refs))
		tt.StopPatterns = append(tt.StopPatterns, refs...)
	}
	tt.StopPatternsOff[numStops] = totalRefs
}

// buildTransfers computes footpaths between stops within max distance + integrates transfers.txt
func (b *Builder) buildTransfers(tt *Timetable, rawTransfers []RawTransfer) {
	numStops := len(tt.Stops)
	transfers := make([][]Transfer, numStops)

	// 1. Same-stop transfer (minimum transfer safety time)
	minTransferSecs := uint16(b.cfg.MinTransferSecs)
	if minTransferSecs == 0 {
		minTransferSecs = 60
	}
	for i := range tt.Stops {
		transfers[i] = append(transfers[i], Transfer{
			To:     StopIdx(i),
			Secs:   minTransferSecs,
			Meters: 0,
		})
	}

	// 2. Spatial footpaths within MaxTransferMeters
	maxMeters := float64(b.cfg.MaxTransferMeters)
	if maxMeters <= 0 {
		maxMeters = 500
	}
	walkSpeed := b.cfg.WalkSpeedMPS
	if walkSpeed <= 0 {
		walkSpeed = 1.33
	}

	for i := 0; i < numStops; i++ {
		s1 := &tt.Stops[i]
		for j := i + 1; j < numStops; j++ {
			s2 := &tt.Stops[j]
			d := haversineMeters(s1.Lat, s1.Lon, s2.Lat, s2.Lon)
			if d <= maxMeters {
				// Detour factor 1.3
				secs := uint16(math.Ceil((d * 1.3) / walkSpeed))
				if secs < minTransferSecs {
					secs = minTransferSecs
				}
				transfers[i] = append(transfers[i], Transfer{To: StopIdx(j), Secs: secs, Meters: uint16(d)})
				transfers[j] = append(transfers[j], Transfer{To: StopIdx(i), Secs: secs, Meters: uint16(d)})
			}
		}
	}

	// 3. Apply transfers.txt overrides
	for _, rt := range rawTransfers {
		from, ok1 := tt.StopByID[rt.FromStopID]
		to, ok2 := tt.StopByID[rt.ToStopID]
		if !ok1 || !ok2 {
			continue
		}
		if rt.TransferType == 3 {
			// Type 3: transfer not possible - remove if exists
			removeTransfer(transfers, from, to)
		} else if rt.TransferType == 2 {
			// Type 2: minimum transfer time specified
			updateOrAddTransfer(transfers, from, to, rt.MinTransferTime)
		}
	}

	tt.TransfersOff = make([]uint32, numStops+1)
	var totalTransfers uint32
	for sIdx, trs := range transfers {
		tt.TransfersOff[sIdx] = totalTransfers
		totalTransfers += uint32(len(trs))
		tt.Transfers = append(tt.Transfers, trs...)
	}
	tt.TransfersOff[numStops] = totalTransfers
}

func removeTransfer(transfers [][]Transfer, from, to StopIdx) {
	list := transfers[from]
	for i, t := range list {
		if t.To == to {
			transfers[from] = append(list[:i], list[i+1:]...)
			return
		}
	}
}

func updateOrAddTransfer(transfers [][]Transfer, from, to StopIdx, minSecs uint16) {
	list := transfers[from]
	for i, t := range list {
		if t.To == to {
			if t.Secs < minSecs {
				transfers[from][i].Secs = minSecs
			}
			return
		}
	}
	transfers[from] = append(transfers[from], Transfer{To: to, Secs: minSecs, Meters: 0})
}

// expandFrequencies unfolds headway-based trips into discrete RawTrips and RawStopTimes.
func (b *Builder) expandFrequencies(trips []RawTrip, stopTimes []RawStopTime, freqs []RawFrequency) ([]RawTrip, []RawStopTime) {
	freqByTrip := make(map[string][]RawFrequency)
	for _, f := range freqs {
		freqByTrip[f.TripID] = append(freqByTrip[f.TripID], f)
	}

	baseStopTimes := make(map[string][]RawStopTime)
	for _, st := range stopTimes {
		baseStopTimes[st.TripID] = append(baseStopTimes[st.TripID], st)
	}

	var newTrips []RawTrip
	var newStopTimes []RawStopTime

	for _, tr := range trips {
		fList, hasFreq := freqByTrip[tr.ID]
		if !hasFreq {
			newTrips = append(newTrips, tr)
			newStopTimes = append(newStopTimes, baseStopTimes[tr.ID]...)
			continue
		}

		sts := baseStopTimes[tr.ID]
		if len(sts) == 0 {
			continue
		}
		sort.Slice(sts, func(i, j int) bool { return sts[i].StopSequence < sts[j].StopSequence })
		baseStart := sts[0].DepTime

		for _, f := range fList {
			if f.HeadwaySecs == 0 {
				continue
			}
			offset := gtfs.Seconds(0)
			step := int(f.HeadwaySecs)
			curStart := f.StartTime

			instIdx := 0
			for curStart <= f.EndTime {
				offset = curStart - baseStart
				instID := fmt.Sprintf("%s_f%d", tr.ID, instIdx)

				newTr := tr
				newTr.ID = instID
				newTrips = append(newTrips, newTr)

				for _, st := range sts {
					newSt := st
					newSt.TripID = instID
					newSt.ArrTime += offset
					newSt.DepTime += offset
					newStopTimes = append(newStopTimes, newSt)
				}

				curStart += gtfs.Seconds(step)
				instIdx++
			}
		}
	}

	return newTrips, newStopTimes
}

func interpolateTimes(sts []RawStopTime) {
	if len(sts) < 2 {
		return
	}
	for i := 0; i < len(sts); i++ {
		if sts[i].ArrTime == gtfs.NoTime && sts[i].DepTime != gtfs.NoTime {
			sts[i].ArrTime = sts[i].DepTime
		} else if sts[i].DepTime == gtfs.NoTime && sts[i].ArrTime != gtfs.NoTime {
			sts[i].DepTime = sts[i].ArrTime
		}
	}
}

func haversineMeters(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadius = 6371000.0 // meters
	dLat := (lat2 - lat1) * (math.Pi / 180.0)
	dLon := (lon2 - lon1) * (math.Pi / 180.0)

	rLat1 := lat1 * (math.Pi / 180.0)
	rLat2 := lat2 * (math.Pi / 180.0)

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(rLat1)*math.Cos(rLat2)*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadius * c
}

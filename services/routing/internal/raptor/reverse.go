package raptor

import (
	"context"

	"github.com/city-connector-backend/routing/internal/gtfs"
	"github.com/city-connector-backend/routing/internal/timetable"
)

// ReverseEngine computes latest departures to arrive at the destination before a target deadline.
type ReverseEngine struct {
	pool *WorkspacePool
}

func NewReverseEngine() *ReverseEngine {
	return &ReverseEngine{
		pool: NewWorkspacePool(),
	}
}

// RouteReverse executes backward RAPTOR (tech spec §6 arriveBy=true).
func (e *ReverseEngine) RouteReverse(ctx context.Context, tt *timetable.Timetable, q Query) []Journey {
	if len(q.AccessStops) == 0 || len(q.EgressStops) == 0 || len(tt.Stops) == 0 {
		return nil
	}

	maxRounds := q.MaxTransfers + 1
	numStops := len(tt.Stops)

	ws := e.pool.Get(maxRounds, numStops)
	defer e.pool.Put(ws)

	// In reverse search:
	// - "Access" (origin) is where we want to MAXIMIZE departure time.
	// - "Egress" (destination) is our starting point at target arrival time T.
	// We initialize tau values to -1 (Earliest/None) and search for MAXIMUM departure.
	for i := range ws.tau {
		ws.tau[i] = -1
	}
	for i := range ws.bestArrival {
		ws.bestArrival[i] = -1
	}

	bestOriginDeparture := gtfs.Seconds(-1)

	// Round 0: Initialize egress stops with target arrival minus walking time
	for _, eg := range q.EgressStops {
		latestArr := q.Departure - eg.WalkSeconds // q.Departure represents target arrival
		if latestArr > ws.bestArrival[eg.StopIdx] {
			ws.SetTau(0, eg.StopIdx, latestArr)
			ws.bestArrival[eg.StopIdx] = latestArr
			ws.markedCurrent.Set(int(eg.StopIdx))
		}
	}

	// Direct walking path between egress and access
	for _, acc := range q.AccessStops {
		t0 := ws.Tau(0, acc.StopIdx)
		if t0 != -1 {
			cand := t0 - acc.WalkSeconds
			if cand > bestOriginDeparture {
				bestOriginDeparture = cand
			}
		}
	}

	// Rounds 1 to maxRounds
	for k := 1; k <= maxRounds; k++ {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		copy(ws.markedPrevious, ws.markedCurrent)
		for i := range ws.markedCurrent {
			ws.markedCurrent[i] = 0
		}

		clear(ws.queuePatterns)
		for s := 0; s < numStops; s++ {
			if !ws.markedPrevious.Has(s) {
				continue
			}
			sIdx := timetable.StopIdx(s)
			startRef := tt.StopPatternsOff[sIdx]
			endRef := tt.StopPatternsOff[sIdx+1]

			for r := startRef; r < endRef; r++ {
				ref := tt.StopPatterns[r]
				// In reverse search, record the MAXIMUM stop index for backward scan
				if pos, exists := ws.queuePatterns[ref.PatternIdx]; !exists || ref.StopPos > pos {
					ws.queuePatterns[ref.PatternIdx] = ref.StopPos
				}
			}
		}

		if len(ws.queuePatterns) == 0 {
			break
		}

		// Step 1: Scan trips backwards along marked patterns
		for pIdx, maxAlightPos := range ws.queuePatterns {
			p := &tt.Patterns[pIdx]
			var currentTripOffset uint32 = ^uint32(0)
			var alightPos uint16

			for pos := int(maxAlightPos); pos >= 0; pos-- {
				sIdx := tt.PatternStops[p.StopsOff+uint32(pos)]

				// If currently holding a trip, update departure time at this upstream stop
				if currentTripOffset != ^uint32(0) {
					st := tt.GetStopTime(p, currentTripOffset, uint16(pos))
					if st.Dep > ws.bestArrival[sIdx] {
						ws.SetTau(k, sIdx, st.Dep)
						ws.bestArrival[sIdx] = st.Dep
						ws.markedCurrent.Set(int(sIdx))

						alightSt := tt.GetStopTime(p, currentTripOffset, alightPos)
						toStop := tt.PatternStops[p.StopsOff+uint32(alightPos)]
						ws.SetLabel(k, sIdx, Label{
							Type:       LabelTransit,
							FromStop:   toStop, // Stores the downstream connected stop for backtracking
							BoardPos:   uint16(pos),
							AlightPos:  alightPos,
							PatternIdx: pIdx,
							TripOffset: currentTripOffset,
							DepTime:    st.Dep,
							ArrTime:    alightSt.Arr,
						})
					}
				}

				// Check if we can catch a LATER trip arriving at or before prevTau
				prevTau := ws.Tau(k-1, sIdx)
				if prevTau != -1 {
					tripOffset := e.findLatestTrip(tt, p, uint16(pos), prevTau, q.ServiceDay)
					if tripOffset != ^uint32(0) {
						if currentTripOffset == ^uint32(0) {
							currentTripOffset = tripOffset
							alightPos = uint16(pos)
						} else {
							candArr := tt.GetStopTime(p, tripOffset, uint16(pos)).Arr
							currArr := tt.GetStopTime(p, currentTripOffset, uint16(pos)).Arr
							if candArr > currArr {
								currentTripOffset = tripOffset
								alightPos = uint16(pos)
							}
						}
					}
				}
			}
		}

		// Step 2: Apply footpaths/transfers backwards
		markedStops := make([]timetable.StopIdx, 0, 32)
		for s := 0; s < numStops; s++ {
			if ws.markedCurrent.Has(s) {
				markedStops = append(markedStops, timetable.StopIdx(s))
			}
		}

		for _, toStop := range markedStops {
			tauTo := ws.Tau(k, toStop)
			startTr := tt.TransfersOff[toStop]
			endTr := tt.TransfersOff[toStop+1]

			for trIdx := startTr; trIdx < endTr; trIdx++ {
				tr := tt.Transfers[trIdx]
				fromStop := tr.To
				if fromStop == toStop {
					continue
				}
				// Walking to reach toStop means we must have departed fromStop by (tauTo - walkSecs)
				depTime := tauTo - gtfs.Seconds(tr.Secs)

				if depTime > ws.bestArrival[fromStop] {
					ws.SetTau(k, fromStop, depTime)
					ws.bestArrival[fromStop] = depTime
					ws.markedCurrent.Set(int(fromStop))

					ws.SetLabel(k, fromStop, Label{
						Type:        LabelTransfer,
						FromStop:    toStop,
						WalkSeconds: tr.Secs,
						WalkMeters:  tr.Meters,
						DepTime:     depTime,
						ArrTime:     tauTo,
					})
				}
			}
		}

		// Update pruning with access walk times
		for _, acc := range q.AccessStops {
			tk := ws.Tau(k, acc.StopIdx)
			if tk != -1 {
				cand := tk - acc.WalkSeconds
				if cand > bestOriginDeparture {
					bestOriginDeparture = cand
				}
			}
		}
	}

	return e.reconstructReverseJourneys(tt, ws, q, maxRounds)
}

func (e *ReverseEngine) findLatestTrip(tt *timetable.Timetable, p *timetable.Pattern, stopPos uint16, maxArr gtfs.Seconds, day gtfs.Date) uint32 {
	// Binary search for latest trip with arr <= maxArr
	low := 0
	high := int(p.NumTrips) - 1
	bestIdx := -1

	for low <= high {
		mid := (low + high) / 2
		st := tt.GetStopTime(p, uint32(mid), stopPos)
		if st.Arr <= maxArr {
			bestIdx = mid
			low = mid + 1 // try later
		} else {
			high = mid - 1
		}
	}

	if bestIdx == -1 {
		return ^uint32(0)
	}

	// Scan backwards for an active trip on this service day
	for i := bestIdx; i >= 0; i-- {
		trip := &tt.Trips[p.TripsOff+uint32(i)]
		if tt.IsServiceActive(trip.ServiceIdx, day) {
			return uint32(i)
		}
	}

	return ^uint32(0)
}

func (e *ReverseEngine) reconstructReverseJourneys(tt *timetable.Timetable, ws *Workspace, q Query, maxRounds int) []Journey {
	var candidates []Journey

	for k := 1; k <= maxRounds; k++ {
		for _, acc := range q.AccessStops {
			tk := ws.Tau(k, acc.StopIdx)
			if tk == -1 {
				continue
			}

			jn := e.forwardTrack(tt, ws, q, k, acc)
			if jn != nil {
				candidates = append(candidates, *jn)
			}
		}
	}

	return FilterPareto(candidates)
}

func (e *ReverseEngine) forwardTrack(tt *timetable.Timetable, ws *Workspace, q Query, round int, access StopArrival) *Journey {
	var legs []JourneyLeg
	currStop := access.StopIdx
	currRound := round

	// Access walk leg
	totalWalk := access.WalkMeters
	depAtOrigin := ws.Tau(round, currStop) - access.WalkSeconds
	if access.WalkSeconds > 0 {
		toS := &tt.Stops[currStop]
		legs = append(legs, JourneyLeg{
			Mode:            LegModeWalk,
			DurationSeconds: int(access.WalkSeconds),
			From:            EndpointPoint{Lat: 0, Lon: 0},
			To:              EndpointPoint{StopID: toS.ID, StopName: toS.Name, Lat: toS.Lat, Lon: toS.Lon},
			DepartAt:        depAtOrigin,
			ArriveAt:        ws.Tau(round, currStop),
			WalkMeters:      access.WalkMeters,
		})
	}

	transfers := 0

	for currRound > 0 {
		lbl := ws.GetLabel(currRound, currStop)
		if lbl.Type == LabelNone {
			break
		}

		if lbl.Type == LabelTransfer {
			totalWalk += int(lbl.WalkMeters)
			fromS := &tt.Stops[currStop]
			toS := &tt.Stops[lbl.FromStop]
			legs = append(legs, JourneyLeg{
				Mode:            LegModeWalk,
				DurationSeconds: int(lbl.WalkSeconds),
				From:            EndpointPoint{StopID: fromS.ID, StopName: fromS.Name, Lat: fromS.Lat, Lon: fromS.Lon},
				To:              EndpointPoint{StopID: toS.ID, StopName: toS.Name, Lat: toS.Lat, Lon: toS.Lon},
				DepartAt:        lbl.DepTime,
				ArriveAt:        lbl.ArrTime,
				WalkMeters:      int(lbl.WalkMeters),
			})
			currStop = lbl.FromStop
			lbl = ws.GetLabel(currRound, currStop)
		}

		if lbl.Type == LabelTransit {
			p := &tt.Patterns[lbl.PatternIdx]
			r := &tt.Routes[p.RouteIdx]
			tr := &tt.Trips[p.TripsOff+lbl.TripOffset]
			boardStop := &tt.Stops[currStop]
			alightStop := &tt.Stops[lbl.FromStop]

			var stopSeq []string
			for pos := lbl.BoardPos; pos <= lbl.AlightPos; pos++ {
				sIdx := tt.PatternStops[p.StopsOff+uint32(pos)]
				stopSeq = append(stopSeq, tt.Stops[sIdx].ID)
			}

			legs = append(legs, JourneyLeg{
				Mode:            LegModeTransit,
				DurationSeconds: int(lbl.ArrTime - lbl.DepTime),
				From:            EndpointPoint{StopID: boardStop.ID, StopName: boardStop.Name, Lat: boardStop.Lat, Lon: boardStop.Lon},
				To:              EndpointPoint{StopID: alightStop.ID, StopName: alightStop.Name, Lat: alightStop.Lat, Lon: alightStop.Lon},
				DepartAt:        lbl.DepTime,
				ArriveAt:        lbl.ArrTime,
				RouteID:         r.ID,
				RouteShortName:  r.ShortName,
				RouteLongName:   r.LongName,
				TripID:          tr.ID,
				Headsign:        tr.Headsign,
				StopSequence:    stopSeq,
			})

			currStop = lbl.FromStop
			currRound--
			transfers++
		}
	}

	// Egress walk leg
	for _, eg := range q.EgressStops {
		if eg.StopIdx == currStop {
			if eg.WalkSeconds > 0 {
				totalWalk += eg.WalkMeters
				lastArr := legs[len(legs)-1].ArriveAt
				fromS := &tt.Stops[currStop]
				legs = append(legs, JourneyLeg{
					Mode:            LegModeWalk,
					DurationSeconds: int(eg.WalkSeconds),
					From:            EndpointPoint{StopID: fromS.ID, StopName: fromS.Name, Lat: fromS.Lat, Lon: fromS.Lon},
					To:              EndpointPoint{Lat: 0, Lon: 0},
					DepartAt:        lastArr,
					ArriveAt:        lastArr + eg.WalkSeconds,
					WalkMeters:      eg.WalkMeters,
				})
			}
			break
		}
	}

	if len(legs) == 0 {
		return nil
	}

	numTransfers := transfers - 1
	if numTransfers < 0 {
		numTransfers = 0
	}

	departAt := legs[0].DepartAt
	arriveAt := legs[len(legs)-1].ArriveAt

	return &Journey{
		DepartAt:        departAt,
		ArriveAt:        arriveAt,
		DurationSeconds: int(arriveAt - departAt),
		Transfers:       numTransfers,
		WalkMeters:      totalWalk,
		Legs:            legs,
	}
}

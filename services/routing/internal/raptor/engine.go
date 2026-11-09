package raptor

import (
	"context"

	"github.com/city-connector-backend/routing/internal/gtfs"
	"github.com/city-connector-backend/routing/internal/timetable"
)

// Engine executes the Round-based Public Transit Routing algorithm (RAPTOR).
type Engine struct {
	pool *WorkspacePool
}

func NewEngine() *Engine {
	return &Engine{
		pool: NewWorkspacePool(),
	}
}

// Route computes candidate Pareto-optimal journeys from access stops to egress stops.
func (e *Engine) Route(ctx context.Context, tt *timetable.Timetable, q Query) []Journey {
	if len(q.AccessStops) == 0 || len(q.EgressStops) == 0 || len(tt.Stops) == 0 {
		return nil
	}

	maxRounds := q.MaxTransfers + 1
	numStops := len(tt.Stops)

	ws := e.pool.Get(maxRounds, numStops)
	defer e.pool.Put(ws)

	// Target pruning: best arrival time to destination egress
	bestTargetArrival := InfinityTime

	// Round 0: Initialize access stops
	for _, acc := range q.AccessStops {
		arrTime := q.Departure + acc.WalkSeconds
		if arrTime < ws.bestArrival[acc.StopIdx] {
			ws.SetTau(0, acc.StopIdx, arrTime)
			ws.bestArrival[acc.StopIdx] = arrTime
			ws.markedCurrent.Set(int(acc.StopIdx))
		}
	}

	// Update bestTargetArrival from Round 0 direct walk if any egress matches access
	for _, eg := range q.EgressStops {
		t0 := ws.Tau(0, eg.StopIdx)
		if t0 != InfinityTime {
			cand := t0 + eg.WalkSeconds
			if cand < bestTargetArrival {
				bestTargetArrival = cand
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

		// Copy markedCurrent to markedPrevious and clear markedCurrent
		copy(ws.markedPrevious, ws.markedCurrent)
		for i := range ws.markedCurrent {
			ws.markedCurrent[i] = 0
		}

		// Accumulate routes/patterns to scan
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
				if pos, exists := ws.queuePatterns[ref.PatternIdx]; !exists || ref.StopPos < pos {
					ws.queuePatterns[ref.PatternIdx] = ref.StopPos
				}
			}
		}

		if len(ws.queuePatterns) == 0 {
			break // Early termination: no reachable routes
		}

		// Step 1: Scan transit trips along each marked pattern
		for pIdx, boardStopPos := range ws.queuePatterns {
			p := &tt.Patterns[pIdx]
			var currentTripOffset uint32 = ^uint32(0)
			var boardPos uint16

			for pos := boardStopPos; pos < p.NumStops; pos++ {
				sIdx := tt.PatternStops[p.StopsOff+uint32(pos)]

				// Update arrival at stop if currently on a trip
				if currentTripOffset != ^uint32(0) {
					st := tt.GetStopTime(p, currentTripOffset, pos)
					if st.Arr < ws.bestArrival[sIdx] && st.Arr < bestTargetArrival {
						ws.SetTau(k, sIdx, st.Arr)
						ws.bestArrival[sIdx] = st.Arr
						ws.markedCurrent.Set(int(sIdx))

						boardSt := tt.GetStopTime(p, currentTripOffset, boardPos)
						fromStop := tt.PatternStops[p.StopsOff+uint32(boardPos)]
						ws.SetLabel(k, sIdx, Label{
							Type:       LabelTransit,
							FromStop:   fromStop,
							BoardPos:   boardPos,
							AlightPos:  pos,
							PatternIdx: pIdx,
							TripOffset: currentTripOffset,
							DepTime:    boardSt.Dep,
							ArrTime:    st.Arr,
						})
					}
				}

				// Can we catch an earlier trip from stop sIdx?
				prevTau := ws.Tau(k-1, sIdx)
				if prevTau != InfinityTime {
					tripOffset := e.findEarliestTrip(tt, p, pos, prevTau, q.ServiceDay)
					if tripOffset != ^uint32(0) {
						if currentTripOffset == ^uint32(0) {
							currentTripOffset = tripOffset
							boardPos = pos
						} else {
							// Compare: is this trip departing earlier/faster than staying on currentTrip?
							candDep := tt.GetStopTime(p, tripOffset, pos).Dep
							currDep := tt.GetStopTime(p, currentTripOffset, pos).Dep
							if candDep < currDep {
								currentTripOffset = tripOffset
								boardPos = pos
							}
						}
					}
				}
			}
		}

		// Step 2: Apply precomputed transfers/footpaths
		markedStops := make([]timetable.StopIdx, 0, 32)
		for s := 0; s < numStops; s++ {
			if ws.markedCurrent.Has(s) {
				markedStops = append(markedStops, timetable.StopIdx(s))
			}
		}

		for _, fromStop := range markedStops {
			tauFrom := ws.Tau(k, fromStop)
			startTr := tt.TransfersOff[fromStop]
			endTr := tt.TransfersOff[fromStop+1]

			for trIdx := startTr; trIdx < endTr; trIdx++ {
				tr := tt.Transfers[trIdx]
				toStop := tr.To
				if toStop == fromStop {
					continue
				}
				arrTime := tauFrom + gtfs.Seconds(tr.Secs)

				if arrTime < ws.bestArrival[toStop] && arrTime < bestTargetArrival {
					ws.SetTau(k, toStop, arrTime)
					ws.bestArrival[toStop] = arrTime
					ws.markedCurrent.Set(int(toStop))

					ws.SetLabel(k, toStop, Label{
						Type:        LabelTransfer,
						FromStop:    fromStop,
						WalkSeconds: tr.Secs,
						WalkMeters:  tr.Meters,
						DepTime:     tauFrom,
						ArrTime:     arrTime,
					})
				}
			}
		}

		// Update target pruning with egress walks
		for _, eg := range q.EgressStops {
			tk := ws.Tau(k, eg.StopIdx)
			if tk != InfinityTime {
				cand := tk + eg.WalkSeconds
				if cand < bestTargetArrival {
					bestTargetArrival = cand
				}
			}
		}
	}

	// Reconstruct itineraries from rounds
	return e.reconstructJourneys(tt, ws, q, maxRounds)
}

func (e *Engine) findEarliestTrip(tt *timetable.Timetable, p *timetable.Pattern, stopPos uint16, minDep gtfs.Seconds, day gtfs.Date) uint32 {
	// Binary search on trips for departure >= minDep
	low := 0
	high := int(p.NumTrips) - 1
	bestIdx := -1

	for low <= high {
		mid := (low + high) / 2
		st := tt.GetStopTime(p, uint32(mid), stopPos)
		if st.Dep >= minDep {
			bestIdx = mid
			high = mid - 1 // try earlier
		} else {
			low = mid + 1
		}
	}

	if bestIdx == -1 {
		return ^uint32(0)
	}

	// Scan forward until we find an active trip on this service day
	for i := bestIdx; i < int(p.NumTrips); i++ {
		trip := &tt.Trips[p.TripsOff+uint32(i)]
		if tt.IsServiceActive(trip.ServiceIdx, day) {
			return uint32(i)
		}
	}

	return ^uint32(0)
}

func (e *Engine) reconstructJourneys(tt *timetable.Timetable, ws *Workspace, q Query, maxRounds int) []Journey {
	var candidates []Journey

	for k := 1; k <= maxRounds; k++ {
		for _, eg := range q.EgressStops {
			tk := ws.Tau(k, eg.StopIdx)
			if tk == InfinityTime {
				continue
			}

			jn := e.backtrack(tt, ws, q, k, eg)
			if jn != nil {
				candidates = append(candidates, *jn)
			}
		}
	}

	return FilterPareto(candidates)
}

func (e *Engine) backtrack(tt *timetable.Timetable, ws *Workspace, q Query, round int, egress StopArrival) *Journey {
	var legs []JourneyLeg
	currStop := egress.StopIdx
	currRound := round

	// Egress leg
	if egress.WalkSeconds > 0 {
		arrTime := ws.Tau(round, currStop) + egress.WalkSeconds
		legs = append(legs, JourneyLeg{
			Mode:            LegModeWalk,
			DurationSeconds: int(egress.WalkSeconds),
			From:            EndpointPoint{StopID: tt.Stops[currStop].ID, StopName: tt.Stops[currStop].Name, Lat: tt.Stops[currStop].Lat, Lon: tt.Stops[currStop].Lon},
			To:              EndpointPoint{Lat: 0, Lon: 0},
			DepartAt:        ws.Tau(round, currStop),
			ArriveAt:        arrTime,
			WalkMeters:      egress.WalkMeters,
		})
	}

	transfers := 0
	totalWalk := egress.WalkMeters

	for currRound > 0 {
		lbl := ws.GetLabel(currRound, currStop)
		if lbl.Type == LabelNone {
			break
		}

		if lbl.Type == LabelTransfer {
			totalWalk += int(lbl.WalkMeters)
			fromS := &tt.Stops[lbl.FromStop]
			toS := &tt.Stops[currStop]
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
			// Same round: transfer occurred after transit leg
			lbl = ws.GetLabel(currRound, currStop)
		}

		if lbl.Type == LabelTransit {
			p := &tt.Patterns[lbl.PatternIdx]
			r := &tt.Routes[p.RouteIdx]
			tr := &tt.Trips[p.TripsOff+lbl.TripOffset]
			boardStop := &tt.Stops[lbl.FromStop]
			alightStop := &tt.Stops[currStop]

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

	// Access leg
	for _, acc := range q.AccessStops {
		if acc.StopIdx == currStop {
			if acc.WalkSeconds > 0 {
				totalWalk += acc.WalkMeters
				firstTransitDep := legs[len(legs)-1].DepartAt
				depTime := firstTransitDep - acc.WalkSeconds
				if depTime < q.Departure {
					depTime = q.Departure
				}
				toS := &tt.Stops[currStop]
				legs = append(legs, JourneyLeg{
					Mode:            LegModeWalk,
					DurationSeconds: int(acc.WalkSeconds),
					From:            EndpointPoint{Lat: 0, Lon: 0},
					To:              EndpointPoint{StopID: toS.ID, StopName: toS.Name, Lat: toS.Lat, Lon: toS.Lon},
					DepartAt:        depTime,
					ArriveAt:        depTime + acc.WalkSeconds,
					WalkMeters:      acc.WalkMeters,
				})
			}
			break
		}
	}

	// Reverse legs slice since we backtracked from destination to origin
	for i, j := 0, len(legs)-1; i < j; i, j = i+1, j-1 {
		legs[i], legs[j] = legs[j], legs[i]
	}

	if len(legs) == 0 {
		return nil
	}

	// Number of transfers = transit legs - 1
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

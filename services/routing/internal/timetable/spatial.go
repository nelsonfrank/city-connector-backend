package timetable

import (
	"math"
)

// SpatialGrid index organizes stops into spatial cells for fast radius / nearest queries.
type SpatialGrid struct {
	cellSizeDeg float64
	cells       map[gridKey][]StopIdx
	stops       []Stop
}

type gridKey struct {
	x int32
	y int32
}

func NewSpatialGrid(stops []Stop, cellMeters float64) *SpatialGrid {
	if cellMeters <= 0 {
		cellMeters = 250.0 // ~250m cells
	}
	// 1 degree latitude ~= 111,000 meters
	cellDeg := cellMeters / 111000.0

	g := &SpatialGrid{
		cellSizeDeg: cellDeg,
		cells:       make(map[gridKey][]StopIdx),
		stops:       stops,
	}

	for idx, s := range stops {
		k := g.key(s.Lat, s.Lon)
		g.cells[k] = append(g.cells[k], StopIdx(idx))
	}
	return g
}

func (g *SpatialGrid) key(lat, lon float64) gridKey {
	return gridKey{
		x: int32(math.Floor(lon / g.cellSizeDeg)),
		y: int32(math.Floor(lat / g.cellSizeDeg)),
	}
}

type StopCandidate struct {
	StopIdx StopIdx
	Meters  float64
}

// FindNearby returns all stops within maxRadiusMeters of (lat, lon), sorted by ascending distance.
func (g *SpatialGrid) FindNearby(lat, lon, maxRadiusMeters float64) []StopCandidate {
	deltaDeg := maxRadiusMeters / 111000.0
	minKey := g.key(lat-deltaDeg, lon-deltaDeg)
	maxKey := g.key(lat+deltaDeg, lon+deltaDeg)

	var candidates []StopCandidate

	for y := minKey.y; y <= maxKey.y; y++ {
		for x := minKey.x; x <= maxKey.x; x++ {
			indices := g.cells[gridKey{x: x, y: y}]
			for _, sIdx := range indices {
				s := &g.stops[sIdx]
				d := haversineMeters(lat, lon, s.Lat, s.Lon)
				if d <= maxRadiusMeters {
					candidates = append(candidates, StopCandidate{
						StopIdx: sIdx,
						Meters:  d,
					})
				}
			}
		}
	}

	return candidates
}

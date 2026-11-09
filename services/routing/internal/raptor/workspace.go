package raptor

import (
	"sync"

	"github.com/city-connector-backend/routing/internal/gtfs"
	"github.com/city-connector-backend/routing/internal/timetable"
)

const InfinityTime gtfs.Seconds = 1<<30 - 1

type LabelType uint8

const (
	LabelNone LabelType = iota
	LabelTransit
	LabelTransfer
)

// Label records how a stop was reached in round k for path reconstruction.
type Label struct {
	Type        LabelType
	FromStop    timetable.StopIdx
	BoardPos    uint16
	AlightPos   uint16
	PatternIdx  timetable.PatternIdx
	TripOffset  uint32 // Offset within pattern's trips
	WalkSeconds uint16
	WalkMeters  uint16
	DepTime     gtfs.Seconds
	ArrTime     gtfs.Seconds
}

// Workspace stores per-query state arrays to avoid allocations.
type Workspace struct {
	MaxRounds int
	NumStops  int

	// tau[round * NumStops + stopIdx] = best arrival at stop in that round
	tau []gtfs.Seconds
	// bestArrival[stopIdx] = minimum arrival time seen across all rounds
	bestArrival []gtfs.Seconds

	// labels[round * NumStops + stopIdx]
	labels []Label

	// Bitsets for marked stops
	markedCurrent  gtfs.Bitset
	markedPrevious gtfs.Bitset

	// Pattern boarding queues: earliest stop index to board pattern p
	queuePatterns map[timetable.PatternIdx]uint16
}

type WorkspacePool struct {
	pool sync.Pool
}

func NewWorkspacePool() *WorkspacePool {
	return &WorkspacePool{}
}

func (p *WorkspacePool) Get(maxRounds, numStops int) *Workspace {
	v := p.pool.Get()
	if v != nil {
		ws := v.(*Workspace)
		if ws.MaxRounds >= maxRounds && ws.NumStops >= numStops {
			ws.Reset(maxRounds, numStops)
			return ws
		}
	}
	return newWorkspace(maxRounds, numStops)
}

func (p *WorkspacePool) Put(ws *Workspace) {
	p.pool.Put(ws)
}

func newWorkspace(maxRounds, numStops int) *Workspace {
	totalSize := (maxRounds + 1) * numStops
	ws := &Workspace{
		MaxRounds:      maxRounds,
		NumStops:       numStops,
		tau:            make([]gtfs.Seconds, totalSize),
		bestArrival:    make([]gtfs.Seconds, numStops),
		labels:         make([]Label, totalSize),
		markedCurrent:  gtfs.NewBitset(numStops),
		markedPrevious: gtfs.NewBitset(numStops),
		queuePatterns:  make(map[timetable.PatternIdx]uint16),
	}
	ws.Reset(maxRounds, numStops)
	return ws
}

func (ws *Workspace) Reset(maxRounds, numStops int) {
	ws.MaxRounds = maxRounds
	ws.NumStops = numStops
	totalSize := (maxRounds + 1) * numStops

	for i := 0; i < totalSize; i++ {
		ws.tau[i] = InfinityTime
		ws.labels[i] = Label{}
	}
	for i := 0; i < numStops; i++ {
		ws.bestArrival[i] = InfinityTime
	}
	for i := range ws.markedCurrent {
		ws.markedCurrent[i] = 0
		ws.markedPrevious[i] = 0
	}
	clear(ws.queuePatterns)
}

func (ws *Workspace) Tau(round int, s timetable.StopIdx) gtfs.Seconds {
	return ws.tau[round*ws.NumStops+int(s)]
}

func (ws *Workspace) SetTau(round int, s timetable.StopIdx, t gtfs.Seconds) {
	ws.tau[round*ws.NumStops+int(s)] = t
}

func (ws *Workspace) GetLabel(round int, s timetable.StopIdx) *Label {
	return &ws.labels[round*ws.NumStops+int(s)]
}

func (ws *Workspace) SetLabel(round int, s timetable.StopIdx, l Label) {
	ws.labels[round*ws.NumStops+int(s)] = l
}

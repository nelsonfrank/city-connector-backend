package timetable

import "sync/atomic"

// Store holds the currently active Timetable and allows lock-free hot swaps
// (blue-green, SAD §6.2).
//
// Readers call Current exactly once per request and use that snapshot for the
// whole request, so in-flight queries finish against the version they started
// with while new queries see the swapped-in version.
type Store struct {
	cur atomic.Pointer[Timetable]
}

// NewStore returns an empty store; Current returns nil until the first Swap.
func NewStore() *Store { return &Store{} }

// Current returns the active timetable, or nil if none has been loaded yet.
func (s *Store) Current() *Timetable { return s.cur.Load() }

// Swap atomically publishes tt as the active timetable and returns the
// previous one (nil on first load). tt must be fully built and validated.
func (s *Store) Swap(tt *Timetable) *Timetable { return s.cur.Swap(tt) }

// Ready reports whether a timetable is loaded, along with its version.
func (s *Store) Ready() (version string, ok bool) {
	tt := s.cur.Load()
	if tt == nil {
		return "", false
	}
	return tt.Version, true
}


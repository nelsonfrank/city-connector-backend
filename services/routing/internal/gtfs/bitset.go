package gtfs

import "math/bits"

// Bitset is a fixed-size set of small non-negative integers. The timetable
// uses it to mark the service dates on which a GTFS service_id is active
// (bit i = horizon start + i days).
type Bitset []uint64

// NewBitset returns a bitset able to hold n bits.
func NewBitset(n int) Bitset { return make(Bitset, (n+63)/64) }

// Set sets bit i. Out-of-range indices are ignored.
func (b Bitset) Set(i int) {
	if i >= 0 && i/64 < len(b) {
		b[i/64] |= 1 << (uint(i) % 64)
	}
}

// Clear clears bit i. Out-of-range indices are ignored.
func (b Bitset) Clear(i int) {
	if i >= 0 && i/64 < len(b) {
		b[i/64] &^= 1 << (uint(i) % 64)
	}
}

// Has reports whether bit i is set. Out-of-range indices report false.
func (b Bitset) Has(i int) bool {
	return i >= 0 && i/64 < len(b) && b[i/64]&(1<<(uint(i)%64)) != 0
}

// Count returns the number of set bits.
func (b Bitset) Count() int {
	n := 0
	for _, w := range b {
		n += bits.OnesCount64(w)
	}
	return n
}


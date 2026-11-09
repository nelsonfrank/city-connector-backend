package timetable

import (
	"sync"
	"testing"
)

func TestStoreSwap(t *testing.T) {
	s := NewStore()
	if s.Current() != nil {
		t.Fatal("new store should be empty")
	}
	if _, ok := s.Ready(); ok {
		t.Fatal("empty store must not be ready")
	}

	v1 := &Timetable{Version: "v1"}
	if prev := s.Swap(v1); prev != nil {
		t.Fatalf("first swap returned %v, want nil", prev)
	}
	if v, ok := s.Ready(); !ok || v != "v1" {
		t.Fatalf("Ready() = %q,%v; want v1,true", v, ok)
	}

	v2 := &Timetable{Version: "v2"}
	if prev := s.Swap(v2); prev != v1 {
		t.Fatal("second swap should return v1")
	}
	if s.Current() != v2 {
		t.Fatal("Current should be v2")
	}
}

// Run with -race: concurrent readers must always observe a complete snapshot.
func TestStoreConcurrentReaders(t *testing.T) {
	s := NewStore()
	s.Swap(&Timetable{Version: "v0"})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				if tt := s.Current(); tt == nil || tt.Version == "" {
					t.Error("observed incomplete snapshot")
					return
				}
			}
		}()
	}
	for j := 0; j < 100; j++ {
		s.Swap(&Timetable{Version: "v"})
	}
	wg.Wait()
}


package timetable

import "context"

// Source defines an interface for fetching raw GTFS data.
type Source interface {
	Load(ctx context.Context) (*RawFeed, error)
	ActiveVersion(ctx context.Context) (string, error)
}

